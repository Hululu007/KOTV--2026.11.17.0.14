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
# fvp 的 dispose 在删除播放器前等待视频尺寸。尺寸 Completer 被换掉后可能永远等不到，
# mdkPlayerAPI_delete 不执行，上一集的包缓存就留在进程里。补丁让释放纹理时不等尺寸，并等待删除完成。
#
# FvpVideoView.surfaceDestroyed 上游会 release()→nativeSetSurface(null)。全屏改
# LayoutParams 常触发临时 surfaceDestroyed，整表抹掉 players 后易定格/崩。改成空
# 实现（与 KotvExo SurfaceView 同语义）；永久卸面仍走 dispose()→release()。
#
# PlatformView 卸面 UAF：nativeSetSurface 清面分支不读 JNI tunnel，只看绑定时写入的
# TexturePlayer.directSurface。真正崩溃是 Dart 先 mdkPlayerAPI_delete，而
# players[surfaceId] 仍包装同一 handle，随后 FvpVideoView.release→setDecoders。
# 补丁：dispose 在 delete 前调 ReleasePlayer，按 handle 卸面并 erase map。
# （误用 release(tunnel=false) 无效，已撤销。）
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
# Windows 插件在 prepare 之前建一个 D3D11 设备，交给 d3d11.device，并设 d3d11.copy=0
# （均为 mdk Global-Options 文档项）。上游 fvp 会写 d3d11.sync.cpu，wiki/mdk.dll 均无此
# 全局选项（仅有环境变量 D3D11_SYNC_CPU 与文档项 d3d11.sync.kmt），故删掉那一行。
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
# 关「硬解直出」：hwdec=auto（MPV 自选），ANGLE 用 D3D11 优先。
# 开「硬解直出」：dxva2 经 dxva2-egl 零拷贝，须 ANGLE D3D9Ex；被拒则软解。软解若仍停在 D3D9
# （与 D3D11 共享纹理）会蓝屏，补丁在 hwdec-current=no 时重建为 D3D11。
# 补丁在建播放器时读 hwdec：含 dxva2（直出）则 D3D9 优先，否则 D3D11 优先。
# FVP / ANGLE 的 D3D11 设备：按 DXGI HEVC 解码能力选适配器（scripts/native/kotv_dxgi_hevc_adapter.h）。
#
# 按平台：Dart 部分各平台共用，总是打；原生部分（fvp 的 Windows / 安卓、window_manager、
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

# fvp（Dart，各平台）
_kotv_patch_py <<'PY'
import re

# fvp：width/height <= 0 时不要等视频尺寸，并等播放器真正删掉。
fvp_root = pkg_or_skip("fvp")
fvp_version = fvp_root.name
tag = f"fvp ({fvp_version})"

player_dart = fvp_root / "lib" / "src" / "player.dart"
mdk_dart = fvp_root / "lib" / "src" / "video_player_mdk.dart"
if not player_dart.is_file() or not mdk_dart.is_file():
    print(f"ERROR: fvp sources missing under {fvp_root}", file=sys.stderr)
    sys.exit(1)

DISPOSE_OLD = """  /// Release resources
  void dispose() async {
    if (_pp == nullptr) {
      textureId.dispose();
      return;
    }
    // await: ensure no player ref in fvp plugin before mdkPlayerAPI_delete() in dart
    await updateTexture(width: -1);
    state = PlaybackState.stopped;"""
# Previous KOTV dispose patch (stop + updateTexture, no ReleasePlayer).
DISPOSE_LEGACY = """  /// Release resources
  Future<void> dispose() async {
    if (_pp == nullptr) {
      textureId.dispose();
      return;
    }
    // KOTV patch (scripts/patch-media-kit.sh): stop before delete so the demux
    // packet cache goes away with the player.
    state = PlaybackState.stopped;
    // await: ensure no player ref in fvp plugin before mdkPlayerAPI_delete() in dart
    await updateTexture(width: -1);"""
# Brief intermediate: ReleasePlayer on all platforms (desktop MissingPluginException).
DISPOSE_RELEASE_ALWAYS = """  /// Release resources
  Future<void> dispose() async {
    if (_pp == nullptr) {
      textureId.dispose();
      return;
    }
    // KOTV patch (scripts/patch-media-kit.sh): stop before delete so the demux
    // packet cache goes away with the player.
    state = PlaybackState.stopped;
    // Texture: ReleaseRT via updateTexture. PlatformView never sets textureId —
    // ReleasePlayer drops native players[surfaceId] while the handle is still live.
    await updateTexture(width: -1);
    await FvpPlatform.instance.releasePlayer(nativeHandle);"""
DISPOSE_NEW = """  /// Release resources
  Future<void> dispose() async {
    if (_pp == nullptr) {
      textureId.dispose();
      return;
    }
    // KOTV patch (scripts/patch-media-kit.sh): stop before delete so the demux
    // packet cache goes away with the player.
    state = PlaybackState.stopped;
    // Texture: ReleaseRT via updateTexture. Android PlatformView never sets
    // textureId — ReleasePlayer drops native players[surfaceId] while alive.
    await updateTexture(width: -1);
    if (Platform.isAndroid) {
      await FvpPlatform.instance.releasePlayer(nativeHandle);
    }"""
# fvp 0.40+: dispose() → unawaited(disposeAsync()); 真正卸面在 disposeAsync。
DISPOSE_ASYNC_OLD = """  /// Release resources and wait for native teardown to finish.
  Future<void> disposeAsync() async {
    if (_pp == nullptr) {
      textureId.dispose();
      return;
    }
    await updateTexture(width: -1);
    state = PlaybackState.stopped;"""
DISPOSE_ASYNC_NEW = """  /// Release resources and wait for native teardown to finish.
  Future<void> disposeAsync() async {
    if (_pp == nullptr) {
      textureId.dispose();
      return;
    }
    // KOTV patch (scripts/patch-media-kit.sh): stop before delete so the demux
    // packet cache goes away with the player.
    state = PlaybackState.stopped;
    // Texture: ReleaseRT via updateTexture. Android PlatformView never sets
    // textureId — ReleasePlayer drops native players[surfaceId] while alive.
    await updateTexture(width: -1);
    if (Platform.isAndroid) {
      await FvpPlatform.instance.releasePlayer(nativeHandle);
    }"""
SIZE_WAIT_OLD = """      textureId.value = null;
    }
    final size = await _videoSize.future;"""
SIZE_WAIT_NEW = """      textureId.value = null;
    }
    // KOTV patch (scripts/patch-media-kit.sh): releasing the texture must not wait
    // for a video size. dispose passes width -1; a replaced _videoSize completer
    // would otherwise never finish and mdkPlayerAPI_delete would not run.
    if ((width != null && width <= 0) || (height != null && height <= 0)) {
      return -1;
    }
    final size = await _videoSize.future;"""
# fvp 0.40+ 已上游化同等逻辑（注释不同）；当作已打补丁。
SIZE_WAIT_UPSTREAM = """      textureId.value = null;
    }
    // Releasing a texture must not wait for media/video size. That future may
    // never complete when an in-flight load is cancelled by disposal.
    if ((width != null && width <= 0) || (height != null && height <= 0)) {
      return -1;
    }
    final size = await _videoSize.future;"""
# 0.39 在删播放器前先清 platform view；Win7 的 0.37.3 没有这一行。
AWAIT_PAIRS = (
    (
        """    _platformViewParams.remove(playerId);
    _players.remove(playerId)?.dispose();""",
        """    _platformViewParams.remove(playerId);
    await _players.remove(playerId)?.dispose();""",
    ),
    (
        """  Future<void> dispose(int playerId) async {
    _players.remove(playerId)?.dispose();
  }""",
        """  Future<void> dispose(int playerId) async {
    await _players.remove(playerId)?.dispose();
  }""",
    ),
)
# Player.dispose 改成 Future 后，子类 MdkVideoPlayer.dispose 仍是 void 就编不过。
SUB_DISPOSE_OLD = """  @override
  void dispose() {
    streamCtl.close();
    _initialized = false;
    super.dispose();
  }"""
SUB_DISPOSE_NEW = """  @override
  Future<void> dispose() async {
    streamCtl.close();
    _initialized = false;
    await super.dispose();
  }"""

player_orig = player_text = player_dart.read_text(encoding="utf-8")
mdk_orig = mdk_text = mdk_dart.read_text(encoding="utf-8")

player_text = one_of(
    tag,
    "dispose stops before delete + ReleasePlayer",
    player_text,
    (
        (DISPOSE_ASYNC_OLD, DISPOSE_ASYNC_NEW),
        (DISPOSE_OLD, DISPOSE_NEW),
        (DISPOSE_LEGACY, DISPOSE_NEW),
        (DISPOSE_RELEASE_ALWAYS, DISPOSE_NEW),
    ),
)
player_text = one_of(
    tag,
    "updateTexture release does not wait for size",
    player_text,
    (
        (SIZE_WAIT_OLD, SIZE_WAIT_NEW),
        (SIZE_WAIT_OLD, SIZE_WAIT_UPSTREAM),
    ),
)
if (
    "await player?.disposeAsync()" in mdk_text
    or "await _players.remove(playerId)?.disposeAsync()" in mdk_text
    or "await _players.remove(playerId)?.dispose()" in mdk_text
):
    print(f"ok {tag} already patched: platform dispose awaits player delete")
else:
    mdk_text = one_of(tag, "platform dispose awaits player delete", mdk_text, AWAIT_PAIRS)
if "await super.disposeAsync()" in mdk_text or (
    "Future<void> dispose() async" in mdk_text and "await super.dispose()" in mdk_text
):
    print(f"ok {tag} already patched: MdkVideoPlayer.dispose returns Future")
else:
    mdk_text = once(tag, "MdkVideoPlayer.dispose returns Future", mdk_text, SUB_DISPOSE_OLD, SUB_DISPOSE_NEW)
# reader.buffering 的 position 是毫秒。getPosition 也按毫秒组装区间。
# 片段带上 reader.buffering，避免和 getPosition 里已是毫秒的区间撞车。
BUF_RANGE_OLD = """      if (ev.category == "reader.buffering") {
        final pos = position;
        final bufLen = buffered();
        streamCtl.add(
          VideoEvent(
            eventType: VideoEventType.bufferingUpdate,
            buffered: [
              DurationRange(
                Duration(microseconds: pos),
                Duration(milliseconds: pos + bufLen),
              ),"""
BUF_RANGE_NEW = """      if (ev.category == "reader.buffering") {
        final pos = position;
        final bufLen = buffered();
        streamCtl.add(
          VideoEvent(
            eventType: VideoEventType.bufferingUpdate,
            buffered: [
              DurationRange(
                Duration(milliseconds: pos),
                Duration(milliseconds: pos + bufLen),
              ),"""
mdk_text = once(tag, "reader.buffering range start is milliseconds", mdk_text, BUF_RANGE_OLD, BUF_RANGE_NEW)

# prepare / 出纹理失败时都写 _players[-hashCode]。同一 key 被覆盖时上一只 player
# 再也 dispose 不到，连播重试/换集会把 mdk 包缓存堆在进程里。
# 助手内部用 failedId 赋值，避免全局替换把助手自己也改成递归调用。
# fvp 0.40+：void dispose() 只是 unawaited(disposeAsync())，必须 await disposeAsync。
_orphan_dispose = (
    "await prev.disposeAsync();"
    if "disposeAsync()" in mdk_text
    else "await prev.dispose();"
)
ORPHAN_HELPER = f"""
  /// KOTV patch: keep at most one failed player under [-hashCode]; dispose the previous.
  Future<void> _storeFailedPlayer(MdkVideoPlayer player) async {{
    final failedId = -hashCode;
    final prev = _players.remove(failedId);
    _players[failedId] = player;
    if (prev != null && !identical(prev, player)) {{
      {_orphan_dispose}
    }}
  }}
"""
# 清掉旧版误补丁（递归助手、重复注释、误挂的 @override）。
mdk_text, n_broken = re.subn(
    r"  @override\n\n  /// KOTV patch: keep at most one failed player[^\n]*\n"
    r"(?:  /// KOTV patch: keep at most one failed player[^\n]*\n)?"
    r"  Future<void> _storeFailedPlayer\(MdkVideoPlayer player\) async \{\n"
    r"    final prev = _players\.remove\(-hashCode\);\n"
    r"    await _storeFailedPlayer\(player\);\n"
    r"    if \(prev != null && !identical\(prev, player\)\) \{\n"
    r"      await prev\.dispose\(\);\n"
    r"    \}\n"
    r"  \}\n+",
    ORPHAN_HELPER.lstrip("\n") + "\n",
    mdk_text,
    count=1,
)
if n_broken:
    print(f"patched fvp ({fvp_version}): fix recursive failed-player helper")

mdk_text, n_dup = re.subn(
    r"  @override\n\n  /// KOTV patch: keep at most one failed player[^\n]*\n"
    r"  /// KOTV patch: keep at most one failed player[^\n]*\n"
    r"  Future<void> _storeFailedPlayer\(MdkVideoPlayer player\) async \{\n"
    r"    final failedId = -hashCode;\n"
    r"    final prev = _players\.remove\(failedId\);\n"
    r"    _players\[failedId\] = player;\n"
    r"    if \(prev != null && !identical\(prev, player\)\) \{\n"
    r"      await prev\.dispose\(\);\n"
    r"    \}\n"
    r"  \}\n+",
    ORPHAN_HELPER.lstrip("\n") + "\n",
    mdk_text,
    count=1,
)
if n_dup:
    print(f"patched fvp ({fvp_version}): clean failed-player helper placement")

if "final failedId = -hashCode;" not in mdk_text:
    anchor = "  Future<void> dispose(int playerId) async {"
    if anchor not in mdk_text:
        print("ERROR: fvp platform dispose anchor missing for orphan helper", file=sys.stderr)
        sys.exit(1)
    mdk_text = mdk_text.replace(anchor, ORPHAN_HELPER + "\n" + anchor, 1)
    print(f"patched fvp ({fvp_version}): failed-player orphan helper")
else:
    print(f"ok fvp ({fvp_version}) already patched: failed-player orphan helper")

orphan_n = mdk_text.count("_players[-hashCode] = player;")
if orphan_n:
    mdk_text = mdk_text.replace(
        "_players[-hashCode] = player;",
        "await _storeFailedPlayer(player);",
    )
    print(f"patched fvp ({fvp_version}): store failed players via helper ({orphan_n})")
elif "await _storeFailedPlayer(player);" in mdk_text:
    print(f"ok fvp ({fvp_version}) already patched: store failed players via helper")
else:
    print(
        f"ERROR: fvp ({fvp_version}) failed-player store sites missing",
        file=sys.stderr,
    )
    sys.exit(1)
helper_body = mdk_text.split("final failedId = -hashCode;", 1)[-1].split(
    "Future<void> dispose(int playerId)", 1
)[0]
if "await _storeFailedPlayer(player);" in helper_body:
    print("ERROR: failed-player helper still calls itself", file=sys.stderr)
    sys.exit(1)
if mdk_text.count("await _storeFailedPlayer(player);") < 3:
    print("ERROR: expected at least 3 failed-player store call sites", file=sys.stderr)
    sys.exit(1)

# 上游可能仍写 d3d11.sync.cpu；缓存里也可能已被旧补丁删掉（注释文案会变）。
if "setGlobalOption('d3d11.sync.cpu'" in mdk_text:
    mdk_text = once(
        tag,
        "remove undocumented d3d11.sync.cpu",
        mdk_text,
        "    mdk.setGlobalOption('d3d11.sync.cpu', 1);\n",
        "    // KOTV patch: upstream fvp sets d3d11.sync.cpu, but mdk Global-Options has\n"
        "    // no such key (only env D3D11_SYNC_CPU / d3d11.sync.kmt). Drop it; Windows\n"
        "    // plugin publishes d3d11.device + d3d11.copy=0 instead.\n",
    )
else:
    print(f"ok fvp ({fvp_version}) already patched: remove undocumented d3d11.sync.cpu")

# create：player 属性写完后显式 setBufferRange(min:0, max:0)。
# max 必须显式 >=0：Dart 默认 max=-1 会把 max/drop 打回默认值。
# 不用 lowLatency（会把 analyzeduration 砍到 100ms，伪扩展名 prepare 易失败）。
BUF_RANGE_CREATE_OLD = """    _playerOpts?.forEach((key, value) {
      player.setProperty(key, value);
    });
    // KOTV: mdk default minMs=1000 → ~1fps on queue underrun. Call the API
    // (same as lowLatency) without shrinking analyzeduration.
    if (_lowLatency <= 0) {
      player.setBufferRange(min: 0);
    }

    if (_decoders != null) {
"""
BUF_RANGE_CREATE_NEW = """    _playerOpts?.forEach((key, value) {
      player.setProperty(key, value);
    });
    // KOTV: min=0 decode ASAP; max=0 → INT64_MAX. Never pass max=-1.
    if (_lowLatency <= 0) {
      player.setBufferRange(min: 0, max: 0);
    }

    if (_decoders != null) {
"""
if "player.setBufferRange(min: 0, max: 0);" in mdk_text:
    print(f"ok fvp ({fvp_version}) already patched: setBufferRange min=0 max=0 after player opts")
elif "player.setBufferRange(min: 0);" in mdk_text and "_lowLatency <= 0" in mdk_text:
    mdk_text = once(
        tag,
        "setBufferRange min=0 max=0 after player opts",
        mdk_text,
        BUF_RANGE_CREATE_OLD,
        BUF_RANGE_CREATE_NEW,
    )
else:
    mdk_text = once(
        tag,
        "setBufferRange min=0 max=0 after player opts",
        mdk_text,
        """    _playerOpts?.forEach((key, value) {
      player.setProperty(key, value);
    });

    if (_decoders != null) {
""",
        BUF_RANGE_CREATE_NEW,
    )

# create：X-KOTV-Prepare-Ms → prepare(startPosition)；勿发往 CDN。
# Mac MPV Media(start:) / mdk prepare(ms) 同思路；避免 prepare(0)+seekTo 触发安卓 AMediaCodec FLUSHING。
# fvp 0.40+：prepare 包在 _untilCancelled 里，并带 cancellation 早退；旧版仍是 await player.prepare()。
PREPARE_HDR_UA_REFERER = """    // KOTV: X-KOTV-Prepare-Ms → prepare(ms); strip before avio.headers (CDN).
    // UA / Referer 走 avio.user_agent / avio.referer，其余进 avio.headers。
    var prepareMs = 0;
    var ua = '';
    var referer = '';
    var headers = '';
    if (dataSource.httpHeaders.isNotEmpty) {
      dataSource.httpHeaders.forEach((key, value) {
        if (key == 'X-KOTV-Prepare-Ms') {
          prepareMs = int.tryParse(value) ?? 0;
          return;
        }
        final low = key.toLowerCase();
        if (low == 'user-agent' || low == 'ua') {
          ua = value;
          return;
        }
        if (low == 'referer' || low == 'referrer') {
          referer = value;
          return;
        }
        headers += '$key: $value\\r\\n';
      });
    }
    if (ua.isNotEmpty) player.setProperty('avio.user_agent', ua);
    if (referer.isNotEmpty) player.setProperty('avio.referer', referer);
    if (headers.isNotEmpty) player.setProperty('avio.headers', headers);
    player.media = uri;
"""
PREPARE_HDR_MID = """    // KOTV: X-KOTV-Prepare-Ms → prepare(ms); strip before avio.headers (CDN).
    // Same idea as Mac MPV Media(start:) / mdk prepare(startPosition).
    var prepareMs = 0;
    if (dataSource.httpHeaders.isNotEmpty) {
      String headers = '';
      dataSource.httpHeaders.forEach((key, value) {
        if (key == 'X-KOTV-Prepare-Ms') {
          prepareMs = int.tryParse(value) ?? 0;
          return;
        }
        headers += '$key: $value\\r\\n';
      });
      if (headers.isNotEmpty) {
        player.setProperty('avio.headers', headers);
      }
    }
    player.media = uri;
"""
PREPARE_HDR_OLD = """    if (dataSource.httpHeaders.isNotEmpty) {
      String headers = '';
      dataSource.httpHeaders.forEach((key, value) {
        headers += '$key: $value\\r\\n';
      });
      player.setProperty('avio.headers', headers);
    }
    player.media = uri;
"""
PREPARE_CALL_LEGACY_OLD = "    int ret = await player.prepare(); // required!\n"
PREPARE_CALL_LEGACY_MID = "    int ret = await player.prepare(position: prepareMs); // required!\n"
PREPARE_CALL_LEGACY_NEW = "    int ret = await player.prepare(position: prepareMs); // required!\n"
PREPARE_CALL_V40_OLD = """    if (cancellation?.isCancelled ?? false) {
      return _finishCancelledCreate(player);
    }
    final preparation = await _untilCancelled(player.prepare(), cancellation);
"""
PREPARE_CALL_V40_MID = """    if (cancellation?.isCancelled ?? false) {
      return _finishCancelledCreate(player);
    }
    final preparation = await _untilCancelled(player.prepare(position: prepareMs), cancellation);
"""
PREPARE_CALL_V40_NEW = """    if (cancellation?.isCancelled ?? false) {
      return _finishCancelledCreate(player);
    }
    final preparation = await _untilCancelled(player.prepare(position: prepareMs), cancellation);
"""
if "avio.user_agent" in mdk_text and "avio.referer" in mdk_text and "prepare(position: prepareMs)" in mdk_text:
    print(f"ok fvp ({fvp_version}) already patched: prepare(startPosition) + avio.user_agent / avio.referer")
else:
    # 先统一 headers 段，再改 prepare 调用（0.40 _untilCancelled / 旧 await）。
    mdk_text = one_of(
        tag,
        "prepare headers + X-KOTV-Prepare-Ms / avio UA+Referer",
        mdk_text,
        (
            (PREPARE_HDR_UA_REFERER, PREPARE_HDR_UA_REFERER),
            (PREPARE_HDR_MID, PREPARE_HDR_UA_REFERER),
            (PREPARE_HDR_OLD, PREPARE_HDR_UA_REFERER),
        ),
    )
    mdk_text = one_of(
        tag,
        "prepare(startPosition) via X-KOTV-Prepare-Ms",
        mdk_text,
        (
            (PREPARE_CALL_V40_NEW, PREPARE_CALL_V40_NEW),
            (PREPARE_CALL_V40_MID, PREPARE_CALL_V40_NEW),
            (PREPARE_CALL_V40_OLD, PREPARE_CALL_V40_NEW),
            (PREPARE_CALL_LEGACY_NEW, PREPARE_CALL_LEGACY_NEW),
            (PREPARE_CALL_LEGACY_MID, PREPARE_CALL_LEGACY_NEW),
            (PREPARE_CALL_LEGACY_OLD, PREPARE_CALL_LEGACY_NEW),
        ),
    )

# lowLatency 分支：setBufferRange(min:0) 省略 max → Dart 默认 max=-1 会把 max/drop
# 打回默认，并可能让后续路径回到 minMs=1000（ADB timediff≈1000ms）。
LOW_LAT_BUF_OLD = """      if (_lowLatency > 1) {
        player.setBufferRange(min: 0, max: 1000, drop: true);
      } else {
        player.setBufferRange(min: 0);
      }
"""
LOW_LAT_BUF_NEW = """      if (_lowLatency > 1) {
        player.setBufferRange(min: 0, max: 1000, drop: true);
      } else {
        // KOTV: never omit max (default -1 resets max/drop; undoes min=0).
        player.setBufferRange(min: 0, max: 0);
      }
"""
if "never omit max (default -1 resets max/drop" in mdk_text:
    print(f"ok fvp ({fvp_version}) already patched: lowLatency setBufferRange max=0")
elif "player.setBufferRange(min: 0);" in mdk_text:
    mdk_text = once(
        tag,
        "lowLatency setBufferRange max=0",
        mdk_text,
        LOW_LAT_BUF_OLD,
        LOW_LAT_BUF_NEW,
    )
else:
    print(f"ok fvp ({fvp_version}): no bare setBufferRange(min: 0) in lowLatency branch")

if player_text != player_orig:
    player_dart.write_text(player_text, encoding="utf-8")
if mdk_text != mdk_orig:
    mdk_dart.write_text(mdk_text, encoding="utf-8")

_player_dispose_chunk = (
    player_dart.read_text(encoding="utf-8").split("Future<void> disposeAsync", 1)[-1][:500]
    if "Future<void> disposeAsync" in player_dart.read_text(encoding="utf-8")
    else player_dart.read_text(encoding="utf-8").split("Future<void> dispose", 1)[-1][:500]
)
if "state = PlaybackState.stopped;" not in _player_dispose_chunk:
    print("ERROR: fvp dispose patch verification failed", file=sys.stderr)
    sys.exit(1)
if "width <= 0" not in player_dart.read_text(encoding="utf-8"):
    print("ERROR: fvp updateTexture patch verification failed", file=sys.stderr)
    sys.exit(1)
mdk_check = mdk_dart.read_text(encoding="utf-8")
if not (
    "await player?.disposeAsync();" in mdk_check
    or "await _players.remove(playerId)?.disposeAsync();" in mdk_check
    or "await _players.remove(playerId)?.dispose();" in mdk_check
):
    print("ERROR: fvp platform dispose patch verification failed", file=sys.stderr)
    sys.exit(1)
if "await super.disposeAsync();" not in mdk_check and "await super.dispose();" not in mdk_check:
    print("ERROR: fvp MdkVideoPlayer.dispose patch verification failed", file=sys.stderr)
    sys.exit(1)
buf_at = mdk_check.find('reader.buffering')
buf_chunk = mdk_check[buf_at:buf_at + 700] if buf_at >= 0 else ""
if "Duration(microseconds: pos)" in buf_chunk or "Duration(milliseconds: pos)" not in buf_chunk:
    print("ERROR: fvp reader.buffering still uses microseconds", file=sys.stderr)
    sys.exit(1)
if "setGlobalOption('d3d11.sync.cpu'" in mdk_check:
    print("ERROR: fvp still sets undocumented d3d11.sync.cpu", file=sys.stderr)
    sys.exit(1)
if "player.setBufferRange(min: 0, max: 0);" not in mdk_check or "_lowLatency <= 0" not in mdk_check:
    print("ERROR: fvp create missing setBufferRange(min: 0, max: 0) after player opts", file=sys.stderr)
    sys.exit(1)
if "X-KOTV-Prepare-Ms" not in mdk_check or "prepare(position: prepareMs)" not in mdk_check:
    print("ERROR: fvp create missing prepare(startPosition) via X-KOTV-Prepare-Ms", file=sys.stderr)
    sys.exit(1)
if "avio.user_agent" not in mdk_check or "avio.referer" not in mdk_check:
    print("ERROR: fvp create missing avio.user_agent / avio.referer", file=sys.stderr)
    sys.exit(1)
if "player.setBufferRange(min: 0);" in mdk_check and "_lowLatency > 1" in mdk_check:
    print("ERROR: fvp lowLatency still has setBufferRange(min: 0) without max", file=sys.stderr)
    sys.exit(1)
fvp_plat = fvp_root / "lib" / "src" / "fvp_platform_interface.dart"
fvp_ch = fvp_root / "lib" / "src" / "fvp_method_channel.dart"
for p in (fvp_plat, fvp_ch):
    if not p.is_file():
        print(f"ERROR: missing {p}", file=sys.stderr)
        sys.exit(1)

PLAT_OLD = """  Future<void> releaseTexture(int playerHandle, int textureId) {
    throw UnimplementedError('releaseTexture() has not been implemented.');
  }

  Future<void> setMixWithOthers(bool mixWithOthers) async {
    throw UnimplementedError('setMixWithOthers() has not been implemented.');
  }
}"""
PLAT_NEW = """  Future<void> releaseTexture(int playerHandle, int textureId) {
    throw UnimplementedError('releaseTexture() has not been implemented.');
  }

  /// Clear native surface bindings for [playerHandle] (PlatformView path).
  Future<void> releasePlayer(int playerHandle) {
    throw UnimplementedError('releasePlayer() has not been implemented.');
  }

  Future<void> setMixWithOthers(bool mixWithOthers) async {
    throw UnimplementedError('setMixWithOthers() has not been implemented.');
  }
}"""
CH_OLD = """  @override
  Future<void> releaseTexture(int playerHandle, int textureId) async {
    await methodChannel.invokeMethod('ReleaseRT', {
      "player": playerHandle,
      "texture": textureId,
    });
  }

  @override
  Future<void> setMixWithOthers(bool mixWithOthers) async {
    await methodChannel.invokeMethod('MixWithOthers', {"value": mixWithOthers});
  }
}"""
CH_NEW = """  @override
  Future<void> releaseTexture(int playerHandle, int textureId) async {
    await methodChannel.invokeMethod('ReleaseRT', {
      "player": playerHandle,
      "texture": textureId,
    });
  }

  @override
  Future<void> releasePlayer(int playerHandle) async {
    await methodChannel.invokeMethod('ReleasePlayer', {
      "player": playerHandle,
    });
  }

  @override
  Future<void> setMixWithOthers(bool mixWithOthers) async {
    await methodChannel.invokeMethod('MixWithOthers', {"value": mixWithOthers});
  }
}"""
plat_text = fvp_plat.read_text(encoding="utf-8")
if "releasePlayer(int playerHandle)" in plat_text:
    print(f"ok fvp ({fvp_version}) already patched: FvpPlatform.releasePlayer")
elif PLAT_OLD in plat_text:
    fvp_plat.write_text(plat_text.replace(PLAT_OLD, PLAT_NEW, 1), encoding="utf-8")
    print(f"patched fvp ({fvp_version}): FvpPlatform.releasePlayer")
else:
    print(
        f"ERROR: fvp ({fvp_version}) FvpPlatform.releasePlayer anchor missing",
        file=sys.stderr,
    )
    sys.exit(1)
ch_text = fvp_ch.read_text(encoding="utf-8")
if "ReleasePlayer" in ch_text:
    print(f"ok fvp ({fvp_version}) already patched: MethodChannel ReleasePlayer")
elif CH_OLD in ch_text:
    fvp_ch.write_text(ch_text.replace(CH_OLD, CH_NEW, 1), encoding="utf-8")
    print(f"patched fvp ({fvp_version}): MethodChannel ReleasePlayer")
else:
    print(
        f"ERROR: fvp ({fvp_version}) MethodChannel ReleasePlayer anchor missing",
        file=sys.stderr,
    )
    sys.exit(1)

# player.dart may have been written earlier in this run; re-read for final checks.
player_check = player_dart.read_text(encoding="utf-8")
_dispose_chunk = (
    player_check.split("Future<void> disposeAsync", 1)[-1][:900]
    if "Future<void> disposeAsync" in player_check
    else player_check.split("Future<void> dispose", 1)[-1][:900]
)
if "releasePlayer(nativeHandle)" not in player_check or "Platform.isAndroid" not in _dispose_chunk:
    print("ERROR: Player.dispose missing Android-gated releasePlayer before mdk delete", file=sys.stderr)
    sys.exit(1)

print(f"ok fvp ({fvp_version}) patches applied ({player_dart})")
PY

# fvp Windows 插件
if _kotv_for_target windows; then
_kotv_patch_py <<'PY'
fvp_root = pkg_or_skip("fvp")
fvp_version = fvp_root.name
tag = f"fvp ({fvp_version})"

# Windows：三张 shared 纹理轮换。renderVideo 画进 Flutter 没拿着的那张，取纹理时只交 handle。
WIN_PLUGIN = fvp_root / "windows" / "fvp_plugin.cpp"
WIN_COPY_OLD = """        D3D11_TEXTURE2D_DESC desc;
        rt->GetDesc(&desc);
        desc.MiscFlags |= D3D11_RESOURCE_MISC_SHARED;
        MS_ENSURE(dev->CreateTexture2D(&desc, nullptr, &tex));

        ComPtr<IDXGIResource> res;
        MS_ENSURE(tex.As(&res));
        HANDLE shared_handle = nullptr;
        MS_ENSURE(res->GetSharedHandle(&shared_handle));
        flt_surface_desc->struct_size = sizeof(FlutterDesktopGpuSurfaceDescriptor);
        flt_surface_desc->handle = shared_handle;// tex.Get();
        //flt_surface_desc->handle = tex.Get(); // eglbind error
        flt_surface_desc->width = flt_surface_desc->visible_width = desc.Width;
        flt_surface_desc->height = flt_surface_desc->visible_height = desc.Height;
        flt_surface_desc->release_context = nullptr;
        flt_surface_desc->release_callback = [](void* release_context) {};
        //flt_surface_desc->format = kFlutterDesktopPixelFormatBGRA8888;
        flt_tex = make_unique<flutter::TextureVariant>(flutter::GpuSurfaceTexture(
            kFlutterDesktopGpuSurfaceTypeDxgiSharedHandle
            //kFlutterDesktopGpuSurfaceTypeD3d11Texture2D
            , [pflt_surface_desc = flt_surface_desc.get(), this](size_t width, size_t height) {
                //printf("ObtainDescriptorCallback %llux%llu. shared_handle_ %p\\n", width, height, shared_handle_); fflush(nullptr);
                //player.renderVideo(); // stutter
                scoped_lock lock(mtx);
                ctx->CopyResource(tex.Get(), rt.Get());
                ctx->Flush();
                return pflt_surface_desc;
            }));
        textureId = texRegistrar->RegisterTexture(flt_tex.get());


        D3D11RenderAPI ra{};
        ra.rtv = rt.Get();
        setRenderAPI(&ra);
        setVideoSurfaceSize(desc.Width, desc.Height);
        setRenderCallback([this, texRegistrar](void*) {
            scoped_lock lock(mtx);
            renderVideo();
            texRegistrar->MarkTextureFrameAvailable(textureId);
            });"""
WIN_COPY_NEW = """        D3D11_TEXTURE2D_DESC desc;
        rt->GetDesc(&desc);
        desc.MiscFlags |= D3D11_RESOURCE_MISC_SHARED;
        shared_tex[0] = rt;
        MS_ENSURE(dev->CreateTexture2D(&desc, nullptr, &shared_tex[1]));
        MS_ENSURE(dev->CreateTexture2D(&desc, nullptr, &shared_tex[2]));
        for (int i = 0; i < 3; ++i) {
            ComPtr<IDXGIResource> res;
            MS_ENSURE(shared_tex[i].As(&res));
            HANDLE shared_handle = nullptr;
            MS_ENSURE(res->GetSharedHandle(&shared_handle));
            flt_surface_desc[i].struct_size = sizeof(FlutterDesktopGpuSurfaceDescriptor);
            flt_surface_desc[i].handle = shared_handle;
            flt_surface_desc[i].width = flt_surface_desc[i].visible_width = desc.Width;
            flt_surface_desc[i].height = flt_surface_desc[i].visible_height = desc.Height;
            flt_surface_desc[i].release_context = nullptr;
            flt_surface_desc[i].release_callback = [](void*) {};
        }
        flt_tex = make_unique<flutter::TextureVariant>(flutter::GpuSurfaceTexture(
            kFlutterDesktopGpuSurfaceTypeDxgiSharedHandle
            , [this](size_t width, size_t height) {
                scoped_lock lock(mtx);
                previous = held;
                held = presented;
                return &flt_surface_desc[presented];
            }));
        textureId = texRegistrar->RegisterTexture(flt_tex.get());


        D3D11RenderAPI ra{};
        ra.rtv = shared_tex[0].Get();
        setRenderAPI(&ra);
        setVideoSurfaceSize(desc.Width, desc.Height);
        setRenderCallback([this, texRegistrar](void*) {
            scoped_lock lock(mtx);
            // KOTV patch (scripts/patch-media-kit.sh): draw into a texture Flutter
            // is not holding. rtv accepts the texture itself. No CopyResource.
            int i = 0;
            for (int k = 0; k < 3; ++k) {
                if (k != held && k != previous) {
                    i = k;
                    break;
                }
            }
            D3D11RenderAPI frame{};
            frame.rtv = shared_tex[i].Get();
            setRenderAPI(&frame);
            renderVideo();
            presented = i;
            texRegistrar->MarkTextureFrameAvailable(textureId);
            });"""
WIN_MEMBERS_OLD = """    unique_ptr<FlutterDesktopGpuSurfaceDescriptor> flt_surface_desc = make_unique<FlutterDesktopGpuSurfaceDescriptor>();
    ComPtr<ID3D11Texture2D> tex;"""
WIN_MEMBERS_NEW = """    FlutterDesktopGpuSurfaceDescriptor flt_surface_desc[3]{};
    ComPtr<ID3D11Texture2D> shared_tex[3];
    int held = 0;
    int previous = 0;
    int presented = 0;"""

if not WIN_PLUGIN.is_file():
    print(f"ERROR: fvp windows plugin missing: {WIN_PLUGIN}", file=sys.stderr)
    sys.exit(1)
win_orig = win_text = WIN_PLUGIN.read_text(encoding="utf-8")
win_text = once(tag, "windows texture is rendered without a copy", win_text, WIN_COPY_OLD, WIN_COPY_NEW)
win_text = once(tag, "windows texture members", win_text, WIN_MEMBERS_OLD, WIN_MEMBERS_NEW)

DEVICE_BODY = """  // KOTV patch (scripts/patch-media-kit.sh): one D3D11 device before prepare.
  // Decoder and Flutter textures share it. d3d11.copy=0 skips the cross-device copy.
  MS_WARN(D3D11CreateDevice(adapter_.Get(), adapter_ ? D3D_DRIVER_TYPE_UNKNOWN : D3D_DRIVER_TYPE_HARDWARE, nullptr, 0, nullptr, 0, D3D11_SDK_VERSION, &dev_, nullptr, &ctx_));
  if (dev_) {
    ComPtr<ID3D10Multithread> mt;
    if (SUCCEEDED(dev_.As(&mt)))
      mt->SetMultithreadProtected(TRUE);
    SetGlobalOption("d3d11.device", static_cast<void*>(dev_.Get()));
    const int d3d_copy = 0;
    SetGlobalOption("d3d11.copy", d3d_copy);
  }
"""
WIN_DEVICE_PAIRS = (
    (
        """FvpPlugin::FvpPlugin(flutter::TextureRegistrar* tr, IDXGIAdapter* adapter)
    : texture_registrar_(tr)
    , adapter_(adapter)
{
  MdkCallbacksStartup();
}""",
        """FvpPlugin::FvpPlugin(flutter::TextureRegistrar* tr, IDXGIAdapter* adapter)
    : texture_registrar_(tr)
    , adapter_(adapter)
{
  MdkCallbacksStartup();
""" + DEVICE_BODY + "}",
    ),
    (
        """FvpPlugin::FvpPlugin(flutter::TextureRegistrar* tr, IDXGIAdapter* adapter)
    : texture_registrar_(tr)
    , adapter_(adapter)
{}""",
        """FvpPlugin::FvpPlugin(flutter::TextureRegistrar* tr, IDXGIAdapter* adapter)
    : texture_registrar_(tr)
    , adapter_(adapter)
{""" + "\n" + DEVICE_BODY + "}",
    ),
)
win_text = one_of(tag, "d3d11 device exists before prepare", win_text, WIN_DEVICE_PAIRS)
win_text = once(
    tag,
    "CreateRT reuses the shared d3d11 device",
    win_text,
    """      MS_WARN(D3D11CreateDevice(adapter_.Get(), adapter_ ? D3D_DRIVER_TYPE_UNKNOWN : D3D_DRIVER_TYPE_HARDWARE, nullptr, 0, nullptr, 0, D3D11_SDK_VERSION, &dev_, nullptr, &ctx_));
      if (!dev_) {
          result->Error("device", "create device failed");
          return;
      }
      ComPtr<ID3D10Multithread> mt;
      if (SUCCEEDED(dev_.As(&mt)))
          mt->SetMultithreadProtected(TRUE);""",
    """      // KOTV patch (scripts/patch-media-kit.sh): device is created in the
      // constructor and published as d3d11.device before prepare.
      if (!dev_) {
          result->Error("device", "create device failed");
          return;
      }""",
)

if win_text != win_orig:
    WIN_PLUGIN.write_text(win_text, encoding="utf-8")

win_check = WIN_PLUGIN.read_text(encoding="utf-8")
if "frame.rtv = shared_tex[i].Get();" not in win_check or "k != held" not in win_check:
    print("ERROR: fvp windows zero-copy texture patch verification failed", file=sys.stderr)
    sys.exit(1)
if "CopyResource(" in win_check:
    print("ERROR: fvp windows plugin still copies the frame", file=sys.stderr)
    sys.exit(1)
if 'SetGlobalOption("d3d11.device"' not in win_check or 'SetGlobalOption("d3d11.copy"' not in win_check:
    print("ERROR: fvp windows plugin does not publish the shared d3d11 device", file=sys.stderr)
    sys.exit(1)
if win_check.count("D3D11CreateDevice(") != 1:
    print("ERROR: fvp windows plugin must create the d3d11 device once", file=sys.stderr)
    sys.exit(1)
PY
fi

# FVP：按 DXGI HEVC 能力选适配器（与 ANGLE 共用 kotv_dxgi_hevc_adapter.h）。
# 勿在脚本里写死 bash $ROOT：MSYS 只转换传给 WinPython 的 argv，heredoc 里的 /d/... 打不开。
if _kotv_for_target windows; then
_kotv_patch_py <<'PY'
import shutil

fvp_root = pkg_or_fail("fvp")
# cfg = …/flutter/.dart_tool/package_config.json → parents[2] = 仓库根
repo_root = cfg.resolve().parents[2]
WIN = fvp_root / "windows"
HDR_SRC = repo_root / "scripts" / "native" / "kotv_dxgi_hevc_adapter.h"
if not HDR_SRC.is_file():
    raise SystemExit(f"ERROR: missing {HDR_SRC}")
shutil.copy2(HDR_SRC, WIN / "kotv_dxgi_hevc_adapter.h")
plugin = WIN / "fvp_plugin.cpp"
text = plugin.read_text(encoding="utf-8")
if "kotv_dxgi_hevc_adapter.h" not in text:
    needle = '#include "fvp_plugin.h"'
    if needle not in text:
        raise SystemExit("ERROR: fvp_plugin.cpp missing include anchor")
    text = text.replace(needle, needle + "\n#include \"kotv_dxgi_hevc_adapter.h\"", 1)
old = """  ComPtr<IDXGIAdapter> adapter = Registrar_GetGraphicsAdapter(registrar, 0);
  if (!adapter)
    clog << "Flutter graphics adapter is not available, texture may be invisible" << endl;
  auto channel ="""
new = """  // KOTV: prefer DXGI adapter with HEVC decode when present (Optimus).
  ComPtr<IDXGIAdapter> adapter;
  adapter.Attach(kotv_dxgi::PickAdapterPreferHevc());
  if (!adapter)
    adapter = Registrar_GetGraphicsAdapter(registrar, 0);
  if (!adapter)
    clog << "Flutter graphics adapter is not available, texture may be invisible" << endl;
  auto channel ="""
if "PickAdapterPreferHevc" not in text:
    if old not in text:
        raise SystemExit("ERROR: fvp RegisterWithRegistrar adapter anchor missing")
    text = text.replace(old, new, 1)
    print(f"patched fvp ({fvp_root.name}): DXGI HEVC adapter pick")
else:
    print(f"ok fvp ({fvp_root.name}): DXGI HEVC adapter pick already patched")
plugin.write_text(text, encoding="utf-8")
PY
fi

# fvp 安卓插件
if _kotv_for_target android; then
_kotv_patch_py <<'PY'
fvp_root = pkg_or_skip("fvp")
fvp_version = fvp_root.name
tag = f"fvp ({fvp_version})"

# Android FvpVideoView：临时 surfaceDestroyed 勿 release（全屏改尺寸会触发）。
fvp_video_view = (
    fvp_root
    / "android"
    / "src"
    / "main"
    / "java"
    / "com"
    / "mediadevkit"
    / "fvp"
    / "FvpVideoView.java"
)
if not fvp_video_view.is_file():
    print(f"ERROR: missing {fvp_video_view}", file=sys.stderr)
    sys.exit(1)
FVP_SURFACE_DESTROYED_OLD = """    @Override
    public void surfaceDestroyed(SurfaceHolder holder) {
        release();
    }"""
FVP_SURFACE_DESTROYED_NEW = """    @Override
    public void surfaceDestroyed(SurfaceHolder holder) {
        // KOTV patch (scripts/patch-media-kit.sh): fullscreen / LayoutParams
        // changes often destroy the Surface temporarily. Do not release() here
        // (that erases native players and freezes or crashes on rebind).
        // Permanent teardown still goes through dispose() → release().
    }"""
fvv_text = fvp_video_view.read_text(encoding="utf-8")
if FVP_SURFACE_DESTROYED_NEW in fvv_text:
    print(f"ok fvp ({fvp_version}) already patched: FvpVideoView.surfaceDestroyed no-op")
elif FVP_SURFACE_DESTROYED_OLD in fvv_text:
    fvp_video_view.write_text(
        fvv_text.replace(FVP_SURFACE_DESTROYED_OLD, FVP_SURFACE_DESTROYED_NEW, 1),
        encoding="utf-8",
    )
    print(f"patched fvp ({fvp_version}): FvpVideoView.surfaceDestroyed no-op")
else:
    print(
        f"ERROR: fvp ({fvp_version}) FvpVideoView.surfaceDestroyed snippet missing; "
        "upstream changed — update scripts/patch-media-kit.sh",
        file=sys.stderr,
    )
    sys.exit(1)
fvv_check = fvp_video_view.read_text(encoding="utf-8")
if "surfaceDestroyed(SurfaceHolder holder) {\n        release();" in fvv_check:
    print("ERROR: FvpVideoView.surfaceDestroyed still calls release()", file=sys.stderr)
    sys.exit(1)
if "dispose() {\n        surfaceView.getHolder().removeCallback(this);\n        release();" not in fvv_check:
    print("ERROR: FvpVideoView.dispose must still call release()", file=sys.stderr)
    sys.exit(1)

# 撤销误判的 tunnel=false：清面路径不读 JNI tunnel（见 fvp_plugin.cpp）。
FVP_RELEASE_WRONG = """    private void release() {
        if (released) {
            return;
        }
        released = true;
        // KOTV: match ReleaseRT — clear with tunnel=false. Passing the bind-time
        // tunnel=true here UAF's on Redmi MediaCodec during PlatformView.dispose.
        FvpPlugin.nativeSetSurface(0, surfaceId, null, -1, -1, false);
    }"""
FVP_RELEASE_UPSTREAM = """    private void release() {
        if (released) {
            return;
        }
        released = true;
        FvpPlugin.nativeSetSurface(0, surfaceId, null, -1, -1, tunnel);
    }"""
fvv_text = fvp_video_view.read_text(encoding="utf-8")
if FVP_RELEASE_UPSTREAM in fvv_text:
    print(f"ok fvp ({fvp_version}): FvpVideoView.release uses upstream tunnel arg")
elif FVP_RELEASE_WRONG in fvv_text:
    fvp_video_view.write_text(
        fvv_text.replace(FVP_RELEASE_WRONG, FVP_RELEASE_UPSTREAM, 1),
        encoding="utf-8",
    )
    print(f"patched fvp ({fvp_version}): revert FvpVideoView.release tunnel=false")
else:
    print(
        f"ERROR: fvp ({fvp_version}) FvpVideoView.release snippet missing; "
        "upstream changed — update scripts/patch-media-kit.sh",
        file=sys.stderr,
    )
    sys.exit(1)
fvv_check = fvp_video_view.read_text(encoding="utf-8")
if "nativeSetSurface(0, surfaceId, null, -1, -1, false);" in fvv_check:
    print("ERROR: FvpVideoView.release still forced tunnel=false", file=sys.stderr)
    sys.exit(1)
if "nativeSetSurface(0, surfaceId, null, -1, -1, tunnel);" not in fvv_check:
    print("ERROR: FvpVideoView.release must pass bind-time tunnel", file=sys.stderr)
    sys.exit(1)

# PlatformView：dispose 在 mdkPlayerAPI_delete 前按 handle 清 native players map。
fvp_plugin_cpp = fvp_root / "android" / "fvp_plugin.cpp"
fvp_plugin_java = (
    fvp_root
    / "android"
    / "src"
    / "main"
    / "java"
    / "com"
    / "mediadevkit"
    / "fvp"
    / "FvpPlugin.java"
)
for p in (fvp_plugin_cpp, fvp_plugin_java):
    if not p.is_file():
        print(f"ERROR: missing {p}", file=sys.stderr)
        sys.exit(1)

CPP_TP_OLD = """    TexturePlayer(jlong handle)
        : mdk::Player(reinterpret_cast<mdkPlayerAPI*>(handle))
    {
    }

    int width = 0;
    int height = 0;
    jobject surface = nullptr;
    void* vo_opaque = nullptr; // can change by TextureRegistry.SurfaceProducer.Callback
    bool directSurface = false; // decoder renders into the surface, no GL renderer
private:
};"""
CPP_TP_NEW = """    TexturePlayer(jlong handle)
        : mdk::Player(reinterpret_cast<mdkPlayerAPI*>(handle))
        , apiHandle(handle)
    {
    }

    jlong apiHandle = 0;
    int width = 0;
    int height = 0;
    jobject surface = nullptr;
    void* vo_opaque = nullptr; // can change by TextureRegistry.SurfaceProducer.Callback
    bool directSurface = false; // decoder renders into the surface, no GL renderer
private:
};"""
CPP_RELEASE_FN = """
extern "C"
JNIEXPORT void JNICALL
Java_com_mediadevkit_fvp_FvpPlugin_nativeReleasePlayer(JNIEnv *env, jobject thiz, jlong player_handle) {
    // KOTV patch (scripts/patch-media-kit.sh): PlatformView never goes through
    // ReleaseRT. Drop every native map entry for this handle while it is still
    // alive — before Dart mdkPlayerAPI_delete — so a later FvpVideoView.release
    // cannot setDecoders/updateNativeSurface on a freed player.
    if (!player_handle) {
        return;
    }
    for (auto it = players.begin(); it != players.end(); ) {
        auto player = it->second;
        if (player->apiHandle != player_handle) {
            ++it;
            continue;
        }
        auto s = player->surface;
        if (player->directSurface) {
            player->setDecoders(mdk::MediaType::Video, {});
        }
        player->updateNativeSurface(nullptr);
        if (s) {
            env->DeleteGlobalRef(s);
        }
        it = players.erase(it);
    }
}
"""
cpp_text = fvp_plugin_cpp.read_text(encoding="utf-8")
if "nativeReleasePlayer" in cpp_text and "apiHandle" in cpp_text:
    print(f"ok fvp ({fvp_version}) already patched: nativeReleasePlayer")
else:
    if CPP_TP_OLD not in cpp_text:
        print(
            f"ERROR: fvp ({fvp_version}) TexturePlayer snippet missing; "
            "upstream changed — update scripts/patch-media-kit.sh",
            file=sys.stderr,
        )
        sys.exit(1)
    cpp_text = cpp_text.replace(CPP_TP_OLD, CPP_TP_NEW, 1)
    if "Java_com_mediadevkit_fvp_FvpPlugin_nativeSetSurfaceSize" not in cpp_text:
        print("ERROR: nativeSetSurfaceSize anchor missing for ReleasePlayer", file=sys.stderr)
        sys.exit(1)
    cpp_text = cpp_text.replace(
        "extern \"C\"\nJNIEXPORT void JNICALL\nJava_com_mediadevkit_fvp_FvpPlugin_nativeSetSurfaceSize",
        CPP_RELEASE_FN.lstrip("\n")
        + "\nextern \"C\"\nJNIEXPORT void JNICALL\nJava_com_mediadevkit_fvp_FvpPlugin_nativeSetSurfaceSize",
        1,
    )
    fvp_plugin_cpp.write_text(cpp_text, encoding="utf-8")
    print(f"patched fvp ({fvp_version}): nativeReleasePlayer")
cpp_check = fvp_plugin_cpp.read_text(encoding="utf-8")
if "nativeReleasePlayer" not in cpp_check or "apiHandle" not in cpp_check:
    print("ERROR: fvp_plugin.cpp missing nativeReleasePlayer", file=sys.stderr)
    sys.exit(1)

JAVA_NATIVE_OLD = """  static native void nativeSetSurface(long playerHandle, long texId, Surface surface, int w, int h, boolean tunnel);

  /*!
    Surface size change (SurfaceHolder.Callback.surfaceChanged). Only the GL
    render path needs it; with "tunnel" the decoder owns the buffer geometry.
   */
  static native void nativeSetSurfaceSize(long texId, int w, int h);"""
JAVA_NATIVE_NEW = """  static native void nativeSetSurface(long playerHandle, long texId, Surface surface, int w, int h, boolean tunnel);

  /*!
    Drop every native players[] entry for this handle (PlatformView surfaceIds).
    Must run before Dart mdkPlayerAPI_delete.
   */
  static native void nativeReleasePlayer(long playerHandle);

  /*!
    Surface size change (SurfaceHolder.Callback.surfaceChanged). Only the GL
    render path needs it; with "tunnel" the decoder owns the buffer geometry.
   */
  static native void nativeSetSurfaceSize(long texId, int w, int h);"""
JAVA_HANDLER_OLD = """    } else if (call.method.equals("ReleaseRT")) {
      final int texId = call.argument("texture"); // 32bit int, 0, 1, 2 .... but SurfaceTexture.id() is long
      final long texId64 = texId; // MUST cast texId to long, otherwise remove() error
      nativeSetSurface(0, texId, null, -1, -1, false);"""
JAVA_HANDLER_NEW = """    } else if (call.method.equals("ReleasePlayer")) {
      final Number h = call.argument("player");
      nativeReleasePlayer(h.longValue());
      result.success(null);
    } else if (call.method.equals("ReleaseRT")) {
      final int texId = call.argument("texture"); // 32bit int, 0, 1, 2 .... but SurfaceTexture.id() is long
      final long texId64 = texId; // MUST cast texId to long, otherwise remove() error
      nativeSetSurface(0, texId, null, -1, -1, false);"""
java_text = fvp_plugin_java.read_text(encoding="utf-8")
if "nativeReleasePlayer" in java_text and 'equals("ReleasePlayer")' in java_text:
    print(f"ok fvp ({fvp_version}) already patched: FvpPlugin ReleasePlayer")
else:
    if JAVA_NATIVE_OLD not in java_text or JAVA_HANDLER_OLD not in java_text:
        print(
            f"ERROR: fvp ({fvp_version}) FvpPlugin ReleasePlayer anchors missing; "
            "upstream changed — update scripts/patch-media-kit.sh",
            file=sys.stderr,
        )
        sys.exit(1)
    java_text = java_text.replace(JAVA_HANDLER_OLD, JAVA_HANDLER_NEW, 1)
    java_text = java_text.replace(JAVA_NATIVE_OLD, JAVA_NATIVE_NEW, 1)
    fvp_plugin_java.write_text(java_text, encoding="utf-8")
    print(f"patched fvp ({fvp_version}): FvpPlugin ReleasePlayer")
java_check = fvp_plugin_java.read_text(encoding="utf-8")
if "nativeReleasePlayer" not in java_check or 'equals("ReleasePlayer")' not in java_check:
    print("ERROR: FvpPlugin.java missing ReleasePlayer", file=sys.stderr)
    sys.exit(1)
PY
fi

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

# --- DXGI HEVC adapter for ANGLE D3D11 textures (same header as FVP) ---
# cfg 来自 argv（MSYS 会转成 Win 路径）；勿用 heredoc 展开的 bash $ROOT。
import shutil
HDR_SRC = cfg.resolve().parents[2] / "scripts" / "native" / "kotv_dxgi_hevc_adapter.h"
if not HDR_SRC.is_file():
    print(f"ERROR: missing {HDR_SRC}", file=sys.stderr)
    sys.exit(1)
shutil.copy2(HDR_SRC, win / "kotv_dxgi_hevc_adapter.h")

cc = win / "angle_surface_manager.cc"
hh = win / "angle_surface_manager.h"
vo = win / "video_output.cc"
text_cc = cc.read_text(encoding="utf-8")
if "kotv_dxgi_hevc_adapter.h" not in text_cc:
    text_cc = text_cc.replace(
        '#include "angle_surface_manager.h"\n',
        '#include "angle_surface_manager.h"\n#include "kotv_dxgi_hevc_adapter.h"\n',
        1,
    )
old_enum = """    } else {
      IDXGIFactory* dxgi = nullptr;
      ::CreateDXGIFactory(__uuidof(IDXGIFactory), (void**)&dxgi);
      // As far as my experience goes, this is the safest approach. Passing NULL
      // (so-called default) seems to cause issues on Windows 7 or maybe some
      // older graphics drivers. First adapter is the default.
      // D3D_DRIVER_TYPE_UNKNOWN| must be passed with manual adapter selection.
      dxgi->EnumAdapters(0, &adapter);
      dxgi->Release();
    }"""
new_enum = """    } else {
      // KOTV: prefer adapter with HEVC decode (Optimus); else first hardware.
      adapter = kotv_dxgi::PickAdapterPreferHevc();
      if (!adapter) {
        IDXGIFactory* dxgi = nullptr;
        ::CreateDXGIFactory(__uuidof(IDXGIFactory), (void**)&dxgi);
        if (dxgi) {
          dxgi->EnumAdapters(0, &adapter);
          dxgi->Release();
        }
      }
    }"""
if "PickAdapterPreferHevc" not in text_cc:
    if old_enum not in text_cc:
        print("ERROR: ANGLE CreateD3DTexture adapter anchor missing", file=sys.stderr)
        sys.exit(1)
    text_cc = text_cc.replace(old_enum, new_enum, 1)
    print(f"patched media_kit_video ({root.name}): ANGLE DXGI HEVC adapter")
else:
    print(f"ok media_kit_video ({root.name}): ANGLE DXGI HEVC adapter already patched")

# Public getter for prefer_d3d9_ (soft-fallback rebuild).
text_h = hh.read_text(encoding="utf-8")
if "bool prefer_d3d9() const" not in text_h:
    if "  void MakeCurrent(bool value);\n" not in text_h:
        print("ERROR: MakeCurrent anchor missing in angle header", file=sys.stderr)
        sys.exit(1)
    text_h = text_h.replace(
        "  void MakeCurrent(bool value);\n",
        "  void MakeCurrent(bool value);\n\n"
        "  bool prefer_d3d9() const { return prefer_d3d9_; }\n",
        1,
    )
    hh.write_text(text_h, encoding="utf-8")

# Soft on D3D9 ANGLE + D3D11 share handle → blue. Rebuild once on D3D11.
text_vo = vo.read_text(encoding="utf-8")
if "MaybeRewriteAngleForSoft" not in text_vo:
    anchor = "void VideoOutput::CheckAndResize() {\n"
    inject = """void VideoOutput::CheckAndResize() {
  // KOTV: dxva2 asked for D3D9; if decode fell to soft, D3D9+D3D11-share is blue — rebuild on D3D11.
  if (surface_manager_ != nullptr && surface_manager_->prefer_d3d9() && render_context_) {
    if (auto cur = mpv_get_property_string(handle_, "hwdec-current")) {
      const std::string s(cur);
      mpv_free(cur);
      if (s == "no") {
        const int32_t w = surface_manager_->width();
        const int32_t h = surface_manager_->height();
        mpv_render_context_free(render_context_);
        render_context_ = nullptr;
        surface_manager_.reset();
        try {
          surface_manager_ = std::make_unique<ANGLESurfaceManager>(w, h, false);
          surface_manager_->MakeCurrent(true);
          mpv_opengl_init_params gl_init_params{
              [](auto, auto name) {
                return reinterpret_cast<void*>(eglGetProcAddress(name));
              },
              nullptr,
          };
          mpv_render_param params[] = {
              {MPV_RENDER_PARAM_API_TYPE, MPV_RENDER_API_TYPE_OPENGL},
              {MPV_RENDER_PARAM_OPENGL_INIT_PARAMS, &gl_init_params},
              {MPV_RENDER_PARAM_INVALID, nullptr},
          };
          if (mpv_render_context_create(&render_context_, handle_, params) == 0) {
            mpv_render_context_set_update_callback(
                render_context_,
                [](void* context) {
                  auto that = reinterpret_cast<VideoOutput*>(context);
                  that->NotifyRender();
                },
                reinterpret_cast<void*>(this));
            std::cout << "media_kit: VideoOutput: rebuilt ANGLE on D3D11 after soft fallback."
                      << std::endl;
          }
        } catch (...) {
          std::cout << "media_kit: VideoOutput: D3D11 ANGLE rebuild failed." << std::endl;
        }
      }
    }
  }
"""
    if text_vo.count(anchor) != 1:
        print("ERROR: CheckAndResize anchor missing/ambiguous", file=sys.stderr)
        sys.exit(1)
    text_vo = text_vo.replace(anchor, inject, 1)
    # Mark helper name for idempotency without adding a real method:
    text_vo = text_vo.replace(
        "rebuilt ANGLE on D3D11 after soft fallback.",
        "rebuilt ANGLE on D3D11 after soft fallback. /* MaybeRewriteAngleForSoft */",
        1,
    )
    vo.write_text(text_vo, encoding="utf-8")
    print(f"patched media_kit_video ({root.name}): soft-fallback D3D11 ANGLE rebuild")
else:
    print(f"ok media_kit_video ({root.name}): soft-fallback D3D11 rebuild already patched")

cc.write_text(text_cc, encoding="utf-8")
PY
