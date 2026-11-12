#!/usr/bin/env bash
# 给 pub-cache 里的 media_kit 打补丁：_setPropertyFlag 用 1 字节 Bool 承载 MPV_FORMAT_FLAG。
#
# mpv 的 MPV_FORMAT_FLAG 是 int（4 字节），bool_set 取 !!flag；media_kit 只 calloc<Bool>(1)，
# 高 3 字节是堆上脏数据。Windows（CoTaskMemAlloc）常非零 → play() 写 pause=false 被 mpv
# 读成 pause=true，表现为「播放中、缓冲在涨、time-pos 不动，点一下暂停才起播」。macOS/Linux calloc 通常整块清零
# 所以不复现。pub.dev 1.2.6 与上游 main 均未修，故在 flutter pub get 之后打补丁。
#
# 另外删掉 media_kit 写死的 network-timeout=5。安卓播放器不设这个选项。
# 5 秒会被 lavf 当成读超时：下一段超过 5 秒没有新字节，已经进缓存的那段播完就停。
#
# 删掉 media_kit 写死的 cache-on-disk=yes。该缓存文件只追加，媒体关掉才删除。
# 直播开播不会改这个选项；点播需要磁盘缓存时由应用在开播前写成 yes。
#
# prepare 失败写入 _players[-hashCode] 时若覆盖同 key，旧 player 会泄漏；改为先 dispose 再挂新的。
#
# Windows 插件原来每帧 CopyResource 进同一张 shared 纹理。Flutter 只在 handle 变化时
# 才放开它，拷贝没有结束信号。补丁改成三张纹理，renderVideo 直接画进 Flutter 没拿着的
# 那张，不再做整帧拷贝。
#
# reader.buffering 把毫秒的 position 装进了 Duration(microseconds:)。改成毫秒。
# getPosition 组装缓冲区间时也用毫秒。
#
# window_manager 0.4.3 有边框进入系统全屏会先 SC_MAXIMIZE。窗口已经最大化后，
# 紧接着的 SetWindowPos 常常改不到尺寸，子窗口收不到新的 WM_SIZE，画面停在最后一帧。
# 有边框分支去掉这次最大化，只去标题栏样式并铺到显示器。无边框仍靠最大化改尺寸。
# 不最大化时 WM_SIZE 不是 SIZE_MAXIMIZED，离开全屏原有的 ForceChildRefresh 不会跑。
# 进/出都 PostMessage 延后同步 Flutter 子窗（同步改会重入 WM_SIZE 卡死）：先对齐客户区，
# 未对齐再延后重试；已对齐才 ±1 逼重绘。不赌固定毫秒。
#
# video_player 的 formatHint 只在 Android 生效。iOS 打开时若判断为 HLS，Dart 写入
# X-KOTV-Asset-Mime，这里把它从 HTTP 头里拿掉，改成 AVURLAsset 的 MIME。
#
# Win7 线（KOTV_WIN7=1）：media_kit_video 的 ANGLE 依次试 D3D11 → D3D11 9_3 → D3D9 → WARP。
# 关「硬解直出」：d3d11va 直出挂不上（实测），走 d3d11va-copy（回读，不依赖 dxva2-egl），ANGLE 用 D3D11。
# 开「硬解直出」：dxva2 经 dxva2-egl 零拷贝，须 ANGLE D3D9Ex；直出被拒（10 位 / Haswell H265）则软解，不要 dxva2-copy。
# 补丁在建播放器时读 hwdec：含 dxva2（直出）则 D3D9 优先，否则 D3D11 优先。
#
# 按平台：Dart 部分各平台共用，总是打；原生部分（window_manager、
# avfoundation、Win7 的 media_kit_video）只在 KOTV_FLUTTER_TARGET 对应的平台打。
# 幂等：已打过直接通过；找不到目标片段且未打过 → 失败（禁止带着 bug 出包）。
# 用法：KOTV_FLUTTER_TARGET=<平台> scripts/patch-media-kit.sh   （flutter-pub-get.sh 末尾自动调用）
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PKG_CFG="$ROOT/flutter/.dart_tool/package_config.json"
[[ -f "$PKG_CFG" ]] || { echo "missing $PKG_CFG (run flutter pub get first)" >&2; exit 1; }

_kotv_python() {
  if command -v python3 >/dev/null 2>&1; then
    python3 "$@"
  elif command -v python >/dev/null 2>&1; then
    python "$@"
  elif command -v py >/dev/null 2>&1; then
    py -3 "$@"
  else
    echo "ERROR: python not found (needed by patch-media-kit.sh)" >&2
    return 1
  fi
}

# 各段补丁共用的 Python 开头：读 package_config.json 找包根目录，以及按片段替换的公共函数。
_KOTV_PKG_ROOT_PY='
import json, pathlib, sys
from urllib.parse import urlparse
from urllib.request import url2pathname

cfg = pathlib.Path(sys.argv[1])
data = json.loads(cfg.read_text(encoding="utf-8"))


def pkg_root(name):
    for pkg in data.get("packages", []):
        if pkg.get("name") != name:
            continue
        uri = pkg.get("rootUri") or ""
        if uri.startswith("file:"):
            return pathlib.Path(url2pathname(urlparse(uri).path))
        return (cfg.parent / uri).resolve()
    return None


def pkg_or_skip(name):
    root = pkg_root(name)
    if root is None:
        print(f"{name} not in package_config.json; skip {name} patch")
        sys.exit(0)
    return root


def pkg_or_fail(name):
    root = pkg_root(name)
    if root is None:
        print(f"ERROR: {name} not in package_config.json", file=sys.stderr)
        sys.exit(1)
    return root


def one_of(tag, label, blob, pairs):
    """pairs 里任一新文已在就不动；否则把恰好出现一次的原文换成新文；都对不上即失败。"""
    for _old, new in pairs:
        if new in blob:
            print(f"ok {tag} already patched: {label}")
            return blob
    for old, new in pairs:
        n = blob.count(old)
        if n == 1:
            print(f"patched {tag}: {label}")
            return blob.replace(old, new, 1)
        if n > 1:
            print(
                f"ERROR: {tag} {label} snippet count {n}; "
                "upstream changed — update scripts/patch-media-kit.sh",
                file=sys.stderr,
            )
            sys.exit(1)
    print(
        f"ERROR: {tag} {label} snippet count 0; "
        "upstream changed — update scripts/patch-media-kit.sh",
        file=sys.stderr,
    )
    sys.exit(1)


def once(tag, label, blob, old, new):
    return one_of(tag, label, blob, ((old, new),))
'

# 从标准输入读一段补丁脚本，接在共用开头后面运行。
_kotv_patch_py() {
  { printf '%s\n' "$_KOTV_PKG_ROOT_PY"; cat; } | _kotv_python - "$PKG_CFG"
}

# KOTV_FLUTTER_TARGET 是当前要出包的平台（android / ios / macos / windows / linux / web），由各打包脚本传入。
# 只打这个平台会编译的原生代码；没传（手动运行）时全打。Dart 部分各平台共用，总是打。
_kotv_for_target() {
  [[ -z "${KOTV_FLUTTER_TARGET:-}" ]] && return 0
  local t
  for t in "$@"; do
    [[ "$KOTV_FLUTTER_TARGET" == "$t" ]] && return 0
  done
  return 1
}

# media_kit（Dart，各平台）
_kotv_patch_py <<'PY'
import re

root = pkg_or_skip("media_kit")
version = root.name

target = root / "lib" / "src" / "player" / "native" / "player" / "real.dart"
if not target.is_file():
    print(f"ERROR: {target} missing", file=sys.stderr)
    sys.exit(1)

text = target.read_text(encoding="utf-8")

BUGGY = "final ptr = calloc<Bool>(1)..value = value;"
FIXED = (
    "// KOTV patch (scripts/patch-media-kit.sh): MPV_FORMAT_FLAG is a C int (4 bytes);\n"
    "    // a 1-byte Bool leaves 3 bytes of heap garbage that mpv reads via !!flag.\n"
    "    final ptr = calloc<Int32>(1)..value = value ? 1 : 0;"
)

changed = False

if FIXED in text:
    print(f"ok media_kit ({version}) already patched: _setPropertyFlag uses Int32")
elif BUGGY not in text:
    print(
        f"ERROR: media_kit ({version}) _setPropertyFlag snippet not found; "
        "upstream changed — update scripts/patch-media-kit.sh",
        file=sys.stderr,
    )
    sys.exit(1)
elif text.count(BUGGY) != 1:
    print(f"ERROR: expected exactly one occurrence of buggy snippet, got {text.count(BUGGY)}", file=sys.stderr)
    sys.exit(1)
else:
    text = text.replace(BUGGY, FIXED, 1)
    changed = True
    print(f"patched media_kit ({version}): _setPropertyFlag Bool(1 byte) -> Int32")

# media_kit 写死 network-timeout=5。安卓不设这个选项，这里直接删掉这行。
TIMEOUT = "        'network-timeout': '5',\n"
NOTE = "      // network-timeout = 5\n"
if NOTE in text:
    text = text.replace(NOTE, "", 1)
    changed = True
    print(f"patched media_kit ({version}): removed network-timeout comment")
if TIMEOUT in text:
    if text.count(TIMEOUT) != 1:
        print(
            f"ERROR: expected exactly one network-timeout assignment, got {text.count(TIMEOUT)}",
            file=sys.stderr,
        )
        sys.exit(1)
    text = text.replace(TIMEOUT, "", 1)
    changed = True
    print(f"patched media_kit ({version}): removed network-timeout=5")
elif "'network-timeout'" in text:
    print(
        f"ERROR: media_kit ({version}) network-timeout present but not the known '5' line; "
        "upstream changed — update scripts/patch-media-kit.sh",
        file=sys.stderr,
    )
    sys.exit(1)
else:
    print(f"ok media_kit ({version}) already patched: network-timeout unset")

# cache-on-disk=yes：只追加的缓存文件，关掉媒体才删除。默认不要打开。
DISK = "        'cache-on-disk': 'yes',\n"
if DISK in text:
    if text.count(DISK) != 1:
        print(
            f"ERROR: expected exactly one cache-on-disk=yes, got {text.count(DISK)}",
            file=sys.stderr,
        )
        sys.exit(1)
    text = text.replace(DISK, "", 1)
    changed = True
    print(f"patched media_kit ({version}): removed cache-on-disk=yes")
elif "'cache-on-disk'" in text:
    print(
        f"ERROR: media_kit ({version}) cache-on-disk present but not the known yes line; "
        "upstream changed — update scripts/patch-media-kit.sh",
        file=sys.stderr,
    )
    sys.exit(1)
else:
    print(f"ok media_kit ({version}) already patched: cache-on-disk unset")

if changed:
    target.write_text(text, encoding="utf-8")

check = target.read_text(encoding="utf-8")
if FIXED not in check or BUGGY in check:
    print("ERROR: _setPropertyFlag patch verification failed", file=sys.stderr)
    sys.exit(1)
if "'network-timeout'" in check:
    print("ERROR: network-timeout still present after patch", file=sys.stderr)
    sys.exit(1)
if "'cache-on-disk'" in check:
    print("ERROR: cache-on-disk still present after patch", file=sys.stderr)
    sys.exit(1)
print(f"ok media_kit ({version}) patches applied ({target})")
PY

# window_manager（Windows）
if _kotv_for_target windows; then
_kotv_patch_py <<'PY'
wm_root = pkg_or_fail("window_manager")
wm_version = wm_root.name
wm_cpp = wm_root / "windows" / "window_manager.cpp"
if not wm_cpp.is_file():
    print(f"ERROR: missing {wm_cpp}", file=sys.stderr)
    sys.exit(1)
WM_MARKER = "bordered enter does not SC_MAXIMIZE"
WM_CHILD = "enter refreshes the Flutter child"
WM_DEFER = "defer Flutter child refresh"
WM_KICK = """    // KOTV patch (scripts/patch-media-kit.sh): leave already calls
    // ForceChildRefresh from WM_SIZE. Enter never did, so the first
    // fullscreen on Windows 7 leaves the picture stuck.
    ForceChildRefresh();
"""
WM_CHILD_BLOCK = """    // KOTV patch (scripts/patch-media-kit.sh): enter refreshes the Flutter child.
    // Bordered fullscreen is not maximized, so leave never sees SIZE_MAXIMIZED.
    ForceChildRefresh();
    last_state = STATE_FULLSCREEN_ENTERED;
"""
WM_DEFER_BLOCK = """    // KOTV patch (scripts/patch-media-kit.sh): defer Flutter child refresh.
    // Bordered fullscreen is not maximized, so leave never sees SIZE_MAXIMIZED.
    // 回到消息循环再改子窗口。在这次调用里同步改会重入 WM_SIZE，界面卡死。
    last_state = STATE_FULLSCREEN_ENTERED;
    ::PostMessage(mainWindow, WM_APP + 0x51, 0, 0);
"""
WM_SAVE_OLD = """    // Save current window information.
    g_maximized_before_fullscreen = ::IsZoomed(mainWindow);
    g_style_before_fullscreen = GetWindowLong(mainWindow, GWL_STYLE);
    ::GetWindowRect(mainWindow, &g_frame_before_fullscreen);"""
WM_SAVE_NEW = """    // Save current window information.
    // 最大化时 GetWindowRect 是屏幕矩形。退出再按它还原，窗口会停在全屏尺寸。
    WINDOWPLACEMENT placement = {};
    placement.length = sizeof(WINDOWPLACEMENT);
    ::GetWindowPlacement(mainWindow, &placement);
    g_maximized_before_fullscreen = placement.showCmd == SW_SHOWMAXIMIZED;
    g_style_before_fullscreen = GetWindowLong(mainWindow, GWL_STYLE);
    if (g_maximized_before_fullscreen) {
      g_frame_before_fullscreen = placement.rcNormalPosition;
    } else {
      ::GetWindowRect(mainWindow, &g_frame_before_fullscreen);
    }"""
WM_EXIT_OLD = """  } else {  // Restore from fullscreen
    if (!g_maximized_before_fullscreen)
      Restore();
    ::SetWindowLongPtr(mainWindow, GWL_STYLE,
                       g_style_before_fullscreen | WS_OVERLAPPEDWINDOW);
    if (::IsZoomed(mainWindow)) {
      // Refresh the parent mainWindow.
      ::SetWindowPos(mainWindow, nullptr, 0, 0, 0, 0,
                     SWP_NOACTIVATE | SWP_NOMOVE | SWP_NOSIZE | SWP_NOZORDER |
                         SWP_FRAMECHANGED);
      auto rect = RECT{};
      ::GetClientRect(mainWindow, &rect);
      auto flutter_view = ::FindWindowEx(mainWindow, nullptr,
                                         kFlutterViewWindowClassName, nullptr);
      ::SetWindowPos(flutter_view, nullptr, rect.left, rect.top,
                     rect.right - rect.left, rect.bottom - rect.top,
                     SWP_NOACTIVATE | SWP_NOZORDER);
      if (g_maximized_before_fullscreen)
        PostMessage(mainWindow, WM_SYSCOMMAND, SC_MAXIMIZE, 0);
    } else {
      ::SetWindowPos(
          mainWindow, nullptr, g_frame_before_fullscreen.left,
          g_frame_before_fullscreen.top,
          g_frame_before_fullscreen.right - g_frame_before_fullscreen.left,
          g_frame_before_fullscreen.bottom - g_frame_before_fullscreen.top,
          SWP_NOACTIVATE | SWP_NOZORDER);
    }
  }
}"""
WM_EXIT_NEW = """  } else {  // Restore from fullscreen
    // 进入时的 SetWindowPos 会清掉 zoomed，不能再靠 IsZoomed 决定还原。
    ::SetWindowLongPtr(mainWindow, GWL_STYLE,
                       g_style_before_fullscreen | WS_OVERLAPPEDWINDOW);
    ::SetWindowPos(
        mainWindow, nullptr, g_frame_before_fullscreen.left,
        g_frame_before_fullscreen.top,
        g_frame_before_fullscreen.right - g_frame_before_fullscreen.left,
        g_frame_before_fullscreen.bottom - g_frame_before_fullscreen.top,
        SWP_NOACTIVATE | SWP_NOZORDER);
    if (g_maximized_before_fullscreen) {
      ::PostMessage(mainWindow, WM_SYSCOMMAND, SC_MAXIMIZE, 0);
    }
    ::PostMessage(mainWindow, WM_APP + 0x51, 0, 0);
  }
}"""
WM_REFRESH_OLD = """void WindowManager::ForceChildRefresh() {
  HWND hWnd = GetWindow(GetMainWindow(), GW_CHILD);

  RECT rect;

  GetWindowRect(hWnd, &rect);
  SetWindowPos(
      hWnd, nullptr, rect.left, rect.top, rect.right - rect.left + 1,
      rect.bottom - rect.top,
      SWP_NOZORDER | SWP_NOOWNERZORDER | SWP_NOMOVE | SWP_FRAMECHANGED);
  SetWindowPos(
      hWnd, nullptr, rect.left, rect.top, rect.right - rect.left,
      rect.bottom - rect.top,
      SWP_NOZORDER | SWP_NOOWNERZORDER | SWP_NOMOVE | SWP_FRAMECHANGED);
}"""
WM_REFRESH_BLIND = """void WindowManager::ForceChildRefresh() {
  HWND main = GetMainWindow();
  if (!main) return;
  RECT rect;
  if (!::GetClientRect(main, &rect)) return;
  const int w = rect.right - rect.left;
  const int h = rect.bottom - rect.top;
  if (w <= 0 || h <= 0) return;
  HWND view = ::FindWindowEx(main, nullptr, kFlutterViewWindowClassName, nullptr);
  if (!view) return;
  // 尺寸没变时子窗口不重画。不用 SWP_FRAMECHANGED，避免重入非客户区计算。
  ::SetWindowPos(view, nullptr, 0, 0, w + 1, h,
                 SWP_NOACTIVATE | SWP_NOZORDER | SWP_NOOWNERZORDER);
  ::SetWindowPos(view, nullptr, 0, 0, w, h,
                 SWP_NOACTIVATE | SWP_NOZORDER | SWP_NOOWNERZORDER);
}"""
WM_REFRESH_SYNC = """bool WindowManager::ForceChildRefresh() {
  HWND main = GetMainWindow();
  if (!main) return false;
  RECT client;
  if (!::GetClientRect(main, &client)) return false;
  const int w = client.right - client.left;
  const int h = client.bottom - client.top;
  if (w <= 0 || h <= 0) return false;
  HWND view = ::FindWindowEx(main, nullptr, kFlutterViewWindowClassName, nullptr);
  if (!view) return false;
  RECT view_rect;
  if (!::GetClientRect(view, &view_rect)) return false;
  const int vw = view_rect.right - view_rect.left;
  const int vh = view_rect.bottom - view_rect.top;
  // KOTV: 先对齐客户区。未对齐返回 false 让调用方延后重试；已对齐再 ±1 逼重绘。
  // 不用 SWP_FRAMECHANGED，避免重入非客户区计算。
  if (vw != w || vh != h) {
    ::SetWindowPos(view, nullptr, 0, 0, w, h,
                   SWP_NOACTIVATE | SWP_NOZORDER | SWP_NOOWNERZORDER);
    RECT after;
    if (!::GetClientRect(view, &after) || after.right - after.left != w ||
        after.bottom - after.top != h) {
      return false;
    }
  }
  ::SetWindowPos(view, nullptr, 0, 0, w + 1, h,
                 SWP_NOACTIVATE | SWP_NOZORDER | SWP_NOOWNERZORDER);
  ::SetWindowPos(view, nullptr, 0, 0, w, h,
                 SWP_NOACTIVATE | SWP_NOZORDER | SWP_NOOWNERZORDER);
  return true;
}"""
WM_REFRESH_DECL_OLD = "  void WindowManager::ForceChildRefresh();"
WM_REFRESH_DECL_NEW = "  bool WindowManager::ForceChildRefresh();"
WM_MAX_OLD = """  if (isFullScreen) {  // Set to fullscreen
    ::SendMessage(mainWindow, WM_SYSCOMMAND, SC_MAXIMIZE, 0);
    if (!is_frameless_) {"""
WM_MAX_NEW = """  if (isFullScreen) {  // Set to fullscreen
    // KOTV patch (scripts/patch-media-kit.sh): bordered enter does not SC_MAXIMIZE.
    if (!is_frameless_) {"""
WM_CLOSE_OLD = """                     SWP_NOOWNERZORDER | SWP_FRAMECHANGED);
    }
  } else {  // Restore from fullscreen"""
WM_CLOSE_NEW = """                     SWP_NOOWNERZORDER | SWP_FRAMECHANGED);
    } else {
      ::SendMessage(mainWindow, WM_SYSCOMMAND, SC_MAXIMIZE, 0);
    }
  } else {  // Restore from fullscreen"""
WM_RESTORE = "  } else {  // Restore from fullscreen"
wm_text = wm_cpp.read_text(encoding="utf-8")
if WM_KICK in wm_text:
    wm_text = wm_text.replace(WM_KICK, "", 1)
if WM_MARKER not in wm_text:
    if WM_MAX_OLD not in wm_text or WM_CLOSE_OLD not in wm_text:
        print(
            f"ERROR: window_manager ({wm_version}) SetFullScreen anchor missing; "
            "upstream changed — update scripts/patch-media-kit.sh",
            file=sys.stderr,
        )
        sys.exit(1)
    wm_text = wm_text.replace(WM_MAX_OLD, WM_MAX_NEW, 1).replace(WM_CLOSE_OLD, WM_CLOSE_NEW, 1)
    print(f"patched window_manager ({wm_version}): bordered enter does not SC_MAXIMIZE")
if WM_CHILD_BLOCK in wm_text:
    wm_text = wm_text.replace(WM_CHILD_BLOCK, WM_DEFER_BLOCK, 1)
    print(f"patched window_manager ({wm_version}): defer Flutter child refresh")
elif WM_DEFER not in wm_text:
    if WM_RESTORE not in wm_text:
        print(
            f"ERROR: window_manager ({wm_version}) fullscreen restore anchor missing",
            file=sys.stderr,
        )
        sys.exit(1)
    wm_text = wm_text.replace(WM_RESTORE, WM_DEFER_BLOCK + WM_RESTORE, 1)
    print(f"patched window_manager ({wm_version}): defer Flutter child refresh")
if WM_SAVE_OLD in wm_text:
    wm_text = wm_text.replace(WM_SAVE_OLD, WM_SAVE_NEW, 1)
    print(f"patched window_manager ({wm_version}): save restored window rect")
elif "rcNormalPosition" not in wm_text:
    print(
        f"ERROR: window_manager ({wm_version}) fullscreen save anchor missing",
        file=sys.stderr,
    )
    sys.exit(1)
if WM_EXIT_OLD in wm_text:
    wm_text = wm_text.replace(WM_EXIT_OLD, WM_EXIT_NEW, 1)
    print(f"patched window_manager ({wm_version}): restore saved window rect")
elif "不能再靠 IsZoomed" not in wm_text:
    print(
        f"ERROR: window_manager ({wm_version}) fullscreen exit anchor missing",
        file=sys.stderr,
    )
    sys.exit(1)
if WM_REFRESH_DECL_OLD in wm_text:
    wm_text = wm_text.replace(WM_REFRESH_DECL_OLD, WM_REFRESH_DECL_NEW, 1)
    print(f"patched window_manager ({wm_version}): ForceChildRefresh returns bool")
elif WM_REFRESH_DECL_NEW not in wm_text:
    print(
        f"ERROR: window_manager ({wm_version}) ForceChildRefresh decl missing",
        file=sys.stderr,
    )
    sys.exit(1)
if "先对齐客户区" not in wm_text:
    if WM_REFRESH_OLD in wm_text:
        wm_text = wm_text.replace(WM_REFRESH_OLD, WM_REFRESH_SYNC, 1)
    elif WM_REFRESH_BLIND in wm_text:
        wm_text = wm_text.replace(WM_REFRESH_BLIND, WM_REFRESH_SYNC, 1)
    else:
        print(
            f"ERROR: window_manager ({wm_version}) ForceChildRefresh body anchor missing",
            file=sys.stderr,
        )
        sys.exit(1)
    print(f"patched window_manager ({wm_version}): sync Flutter child to client")
wm_cpp.write_text(wm_text, encoding="utf-8")
wm_plugin = wm_root / "windows" / "window_manager_plugin.cpp"
plugin = wm_plugin.read_text(encoding="utf-8")
plugin_leave_old = """      window_manager->ForceChildRefresh();
      _EmitEvent("leave-full-screen");"""
plugin_leave_new = """      _EmitEvent("leave-full-screen");"""
if plugin_leave_old in plugin:
    plugin = plugin.replace(plugin_leave_old, plugin_leave_new, 1)
    print(f"patched window_manager plugin ({wm_version}): leave does not resize inline")
PLUGIN_REFRESH_ONCE = """  if (message == WM_APP + 0x51) {
    window_manager->ForceChildRefresh();
    return 0;
  }
"""
PLUGIN_REFRESH_TIMER = """  if (message == WM_APP + 0x51) {
    window_manager->ForceChildRefresh();
    // ANGLE/纹理可能在这次尺寸变更之后才落稳；再延后踢一次，避免 Win7 偶发定格。
    ::SetTimer(hWnd, 0x4B51, 120, nullptr);
    return 0;
  }
  if (message == WM_TIMER && wParam == 0x4B51) {
    ::KillTimer(hWnd, 0x4B51);
    window_manager->ForceChildRefresh();
    return 0;
  }
"""
PLUGIN_REFRESH_SYNC = """  if (message == WM_APP + 0x51) {
    // 未对齐客户区则延后重试；对齐后再用 0x4B52 确认踢一次（ANGLE/纹理可能更晚落稳）。
    if (!window_manager->ForceChildRefresh()) {
      if (wParam < 8) {
        ::PostMessage(hWnd, WM_APP + 0x51, wParam + 1, 0);
      }
    } else {
      ::SetTimer(hWnd, 0x4B52, 120, nullptr);
    }
    return 0;
  }
  if (message == WM_TIMER && wParam == 0x4B52) {
    ::KillTimer(hWnd, 0x4B52);
    window_manager->ForceChildRefresh();
    return 0;
  }
"""
if "0x4B52" not in plugin:
    if PLUGIN_REFRESH_TIMER in plugin:
        plugin = plugin.replace(PLUGIN_REFRESH_TIMER, PLUGIN_REFRESH_SYNC, 1)
        print(f"patched window_manager plugin ({wm_version}): condition child sync + confirm")
    elif PLUGIN_REFRESH_ONCE in plugin:
        plugin = plugin.replace(PLUGIN_REFRESH_ONCE, PLUGIN_REFRESH_SYNC, 1)
        print(f"patched window_manager plugin ({wm_version}): condition child sync + confirm")
    elif "wParam < 8" in plugin and "0x4B52" not in plugin:
        # 旧条件同步：整段换成带确认踢的 SYNC。
        old_sync = """  if (message == WM_APP + 0x51) {
    // 未对齐客户区则延后重试；对齐后 ForceChildRefresh 内再 ±1 重绘。
    if (!window_manager->ForceChildRefresh() && wParam < 8) {
      ::PostMessage(hWnd, WM_APP + 0x51, wParam + 1, 0);
    }
    return 0;
  }
"""
        if old_sync in plugin:
            plugin = plugin.replace(old_sync, PLUGIN_REFRESH_SYNC, 1)
            print(f"patched window_manager plugin ({wm_version}): add confirm kick after align")
        else:
            plugin_anchor = "  std::optional<LRESULT> result = std::nullopt;\n"
            if plugin_anchor not in plugin:
                print(
                    f"ERROR: window_manager plugin ({wm_version}) HandleWindowProc anchor missing",
                    file=sys.stderr,
                )
                sys.exit(1)
            plugin = plugin.replace(
                plugin_anchor,
                "  std::optional<LRESULT> result = std::nullopt;\n\n" + PLUGIN_REFRESH_SYNC,
                1,
            )
            print(f"patched window_manager plugin ({wm_version}): condition child sync + confirm")
    else:
        plugin_anchor = "  std::optional<LRESULT> result = std::nullopt;\n"
        if plugin_anchor not in plugin:
            print(
                f"ERROR: window_manager plugin ({wm_version}) HandleWindowProc anchor missing",
                file=sys.stderr,
            )
            sys.exit(1)
        plugin = plugin.replace(
            plugin_anchor,
            "  std::optional<LRESULT> result = std::nullopt;\n\n" + PLUGIN_REFRESH_SYNC,
            1,
        )
        print(f"patched window_manager plugin ({wm_version}): condition child sync + confirm")
elif "0x4B51" in plugin:
    # 旧盲踢 timer 残留：换成确认踢 SYNC。
    if PLUGIN_REFRESH_TIMER in plugin:
        plugin = plugin.replace(PLUGIN_REFRESH_TIMER, PLUGIN_REFRESH_SYNC, 1)
        print(f"patched window_manager plugin ({wm_version}): drop blind SetTimer kick")
wm_plugin.write_text(plugin, encoding="utf-8")
wm_check = wm_cpp.read_text(encoding="utf-8")
anchor = "} else {  // Restore from fullscreen"
idx = wm_check.find(anchor)
enter = wm_check[max(0, idx - 1800):idx] if idx >= 0 else ""
if WM_MARKER not in enter or WM_DEFER not in enter or "WM_APP + 0x51" not in enter:
    print("ERROR: window_manager enter-fullscreen missing deferred child refresh", file=sys.stderr)
    sys.exit(1)
if "ForceChildRefresh();" in enter:
    print("ERROR: window_manager enter-fullscreen still refreshes inline", file=sys.stderr)
    sys.exit(1)
if "先对齐客户区" not in wm_check or "bool WindowManager::ForceChildRefresh() {" not in wm_check:
    print("ERROR: window_manager ForceChildRefresh is not condition-driven", file=sys.stderr)
    sys.exit(1)
if "::SendMessage(mainWindow, WM_SYSCOMMAND, SC_MAXIMIZE, 0);" not in enter:
    print("ERROR: window_manager frameless fullscreen lost SC_MAXIMIZE", file=sys.stderr)
    sys.exit(1)
if enter.find("::SendMessage(mainWindow, WM_SYSCOMMAND, SC_MAXIMIZE, 0);") < enter.find("if (!is_frameless_)"):
    print("ERROR: window_manager bordered fullscreen still maximizes first", file=sys.stderr)
    sys.exit(1)
plugin_check = wm_plugin.read_text(encoding="utf-8")
if "message == WM_APP + 0x51" not in plugin_check or "wParam < 8" not in plugin_check:
    print("ERROR: window_manager plugin missing condition child sync", file=sys.stderr)
    sys.exit(1)
if "0x4B52" not in plugin_check:
    print("ERROR: window_manager plugin missing confirm kick after align", file=sys.stderr)
    sys.exit(1)
if "0x4B51" in plugin_check:
    print("ERROR: window_manager plugin still has blind SetTimer kick", file=sys.stderr)
    sys.exit(1)
if plugin_leave_old in plugin_check:
    print("ERROR: window_manager plugin still resizes inside WM_SIZE", file=sys.stderr)
    sys.exit(1)
PY
fi

# video_player_avfoundation（iOS / macOS）
if _kotv_for_target ios macos; then
_kotv_patch_py <<'PY'
root = pkg_or_skip("video_player_avfoundation")
candidates = list(root.rglob("VideoPlayerPlugin.swift"))
if not candidates:
    # Win7 线（Dart 3.3）只能解出 Objective-C 版的 avfoundation，那条线不出 iOS 包。
    print(f"skip avfoundation mime hint: no VideoPlayerPlugin.swift under {root}")
    sys.exit(0)
swift = candidates[0]
text = swift.read_text(encoding="utf-8")
if "X-KOTV-Asset-Mime" in text:
    print(f"avfoundation mime hint already patched ({swift})")
    sys.exit(0)
old = """  @MainActor private func playerItem(with options: CreationOptions) throws -> WrappedAVPlayerItem {
    let headers = options.httpHeaders
    let itemOptions = headers.isEmpty ? nil : ["AVURLAssetHTTPHeaderFieldsKey": headers]
    guard let url = URL(string: options.uri) else {
      throw PigeonError(code: "video_player", message: "Invalid URI", details: nil)
    }
    let asset = avFactory.urlAsset(with: url, options: itemOptions)
    return avFactory.playerItem(with: asset)
  }"""
new = """  @MainActor private func playerItem(with options: CreationOptions) throws -> WrappedAVPlayerItem {
    var headers = options.httpHeaders
    let assetMime = headers.removeValue(forKey: "X-KOTV-Asset-Mime")
    var itemOptions: [String: Any] = [:]
    if !headers.isEmpty {
      itemOptions["AVURLAssetHTTPHeaderFieldsKey"] = headers
    }
    if let assetMime, !assetMime.isEmpty {
      itemOptions["AVURLAssetOverrideMIMETypeKey"] = assetMime
    }
    let opts: [String: Any]? = itemOptions.isEmpty ? nil : itemOptions
    guard let url = URL(string: options.uri) else {
      throw PigeonError(code: "video_player", message: "Invalid URI", details: nil)
    }
    let asset = avFactory.urlAsset(with: url, options: opts)
    return avFactory.playerItem(with: asset)
  }"""
if text.count(old) != 1:
    print(
        f"ERROR: avfoundation ({root.name}) playerItem anchor missing; "
        "upstream changed — update scripts/patch-media-kit.sh",
        file=sys.stderr,
    )
    sys.exit(1)
swift.write_text(text.replace(old, new, 1), encoding="utf-8")
print(f"patched avfoundation mime hint ({swift})")
PY
fi

# media_kit_video（Win7 线，只出 Windows 包）
[[ "${KOTV_WIN7:-}" == "1" ]] || exit 0

_kotv_patch_py <<'PY'
root = pkg_or_fail("media_kit_video")
win = root / "windows"

display_stock = """      display_ = eglGetPlatformDisplayEXT(EGL_PLATFORM_ANGLE_ANGLE,
                                          EGL_DEFAULT_DISPLAY,
                                          kD3D11DisplayAttributes);
      if (eglInitialize(display_, 0, 0) == EGL_FALSE) {
        display_ = eglGetPlatformDisplayEXT(EGL_PLATFORM_ANGLE_ANGLE,
                                            EGL_DEFAULT_DISPLAY,
                                            kD3D11_9_3DisplayAttributes);
        if (eglInitialize(display_, 0, 0) == EGL_FALSE) {
          display_ = eglGetPlatformDisplayEXT(EGL_PLATFORM_ANGLE_ANGLE,
                                              EGL_DEFAULT_DISPLAY,
                                              kD3D9DisplayAttributes);
          if (eglInitialize(display_, 0, 0) == EGL_FALSE) {
            display_ = eglGetPlatformDisplayEXT(EGL_PLATFORM_ANGLE_ANGLE,
                                                EGL_DEFAULT_DISPLAY,
                                                kWrapDisplayAttributes);
            if (eglInitialize(display_, 0, 0) == EGL_FALSE) {
              FAIL("eglGetPlatformDisplayEXT");
            }
          }
        }
      }
"""
# 之前出过的包写死了 D3D9 优先；CI 的 pub 缓存里可能还是它。
display_d3d9_first = """      // KOTV patch (scripts/patch-media-kit.sh, Win7): D3D9Ex first so mpv's
      // dxva2-egl interop can share DXVA2 surfaces with ANGLE.
      const EGLint* attributes[] = {
          kD3D9DisplayAttributes,
          kD3D11DisplayAttributes,
          kD3D11_9_3DisplayAttributes,
          kWrapDisplayAttributes,
      };
      for (auto attribute : attributes) {
        display_ = eglGetPlatformDisplayEXT(EGL_PLATFORM_ANGLE_ANGLE,
                                            EGL_DEFAULT_DISPLAY, attribute);
        if (eglInitialize(display_, 0, 0) == EGL_TRUE) {
          break;
        }
        display_ = EGL_NO_DISPLAY;
      }
      if (display_ == EGL_NO_DISPLAY) {
        FAIL("eglGetPlatformDisplayEXT");
      }
"""
display_new = """      // KOTV patch (scripts/patch-media-kit.sh, Win7): D3D9Ex first when the
      // player decodes with dxva2 directly, so mpv's dxva2-egl interop can share
      // DXVA2 surfaces with ANGLE; otherwise the stock order, D3D11 first.
      const EGLint* d3d9_first[] = {
          kD3D9DisplayAttributes,
          kD3D11DisplayAttributes,
          kD3D11_9_3DisplayAttributes,
          kWrapDisplayAttributes,
      };
      const EGLint* d3d11_first[] = {
          kD3D11DisplayAttributes,
          kD3D11_9_3DisplayAttributes,
          kD3D9DisplayAttributes,
          kWrapDisplayAttributes,
      };
      for (auto attribute : prefer_d3d9_ ? d3d9_first : d3d11_first) {
        display_ = eglGetPlatformDisplayEXT(EGL_PLATFORM_ANGLE_ANGLE,
                                            EGL_DEFAULT_DISPLAY, attribute);
        if (eglInitialize(display_, 0, 0) == EGL_TRUE) {
          break;
        }
        display_ = EGL_NO_DISPLAY;
      }
      if (display_ == EGL_NO_DISPLAY) {
        FAIL("eglGetPlatformDisplayEXT");
      }
"""

# (文件, [(可能的原文…), 新文]) ；同一文件可有多处。
edits = {
    "angle_surface_manager.h": [
        (("  ANGLESurfaceManager(int32_t width, int32_t height);\n",),
         "  ANGLESurfaceManager(int32_t width, int32_t height, bool prefer_d3d9);\n"),
        (("  int32_t height_ = 1;\n",),
         "  int32_t height_ = 1;\n  bool prefer_d3d9_ = false;\n"),
    ],
    "angle_surface_manager.cc": [
        (("ANGLESurfaceManager::ANGLESurfaceManager(int32_t width, int32_t height)\n"
          "    : width_(width), height_(height) {\n",),
         "ANGLESurfaceManager::ANGLESurfaceManager(int32_t width,\n"
         "                                         int32_t height,\n"
         "                                         bool prefer_d3d9)\n"
         "    : width_(width), height_(height), prefer_d3d9_(prefer_d3d9) {\n"),
        ((display_stock, display_d3d9_first), display_new),
    ],
    "video_output.cc": [
        (("        surface_manager_ = std::make_unique<ANGLESurfaceManager>(\n"
          "            static_cast<int32_t>(width_.value_or(1)),\n"
          "            static_cast<int32_t>(height_.value_or(1)));\n",),
         "        // KOTV patch (scripts/patch-media-kit.sh, Win7): the ANGLE backend\n"
         "        // follows this player's hwdec, which Dart sets before\n"
         "        // VideoOutputManager.Create: dxva2 (direct output) needs D3D9.\n"
         "        auto prefer_d3d9 = false;\n"
         "        if (auto hwdec = mpv_get_property_string(handle_, \"hwdec\")) {\n"
         "          prefer_d3d9 = (\",\" + std::string(hwdec) + \",\").find(\",dxva2,\") !=\n"
         "                        std::string::npos;\n"
         "          mpv_free(hwdec);\n"
         "        }\n"
         "        surface_manager_ = std::make_unique<ANGLESurfaceManager>(\n"
         "            static_cast<int32_t>(width_.value_or(1)),\n"
         "            static_cast<int32_t>(height_.value_or(1)), prefer_d3d9);\n"),
    ],
}

for name, pairs in edits.items():
    path = win / name
    if not path.is_file():
        print(f"ERROR: {path} missing", file=sys.stderr)
        sys.exit(1)
    text = path.read_text(encoding="utf-8")
    if "prefer_d3d9" in text:
        print(f"ok media_kit_video ({root.name}) {name}: ANGLE backend by hwdec already patched")
        continue
    for olds, new in pairs:
        hit = next((o for o in olds if text.count(o) == 1), None)
        if hit is None:
            print(
                f"ERROR: media_kit_video ({root.name}) {name} anchor missing; "
                "upstream changed — update scripts/patch-media-kit.sh",
                file=sys.stderr,
            )
            sys.exit(1)
        text = text.replace(hit, new, 1)
    path.write_text(text, encoding="utf-8")
    print(f"patched media_kit_video ({root.name}) {name}: ANGLE backend by hwdec")
PY
