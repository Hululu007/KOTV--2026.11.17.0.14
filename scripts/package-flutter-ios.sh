#!/usr/bin/env bash
# 未签名 iOS IPA。flutter build ipa --no-codesign 不会导出 IPA，
# 且归档产物里的动态库经常是指向 DerivedData 的符号链接，zip -y 会把链接打进包、真身留在外面。
# 这里把设备端框架收成包内的实体文件，缺 mdk / Mpv / App 就失败。
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=kotv-release-name.sh
source "$ROOT/scripts/kotv-release-name.sh"
# shellcheck source=kotv-flutter-asset-engines.sh
source "$ROOT/scripts/kotv-flutter-asset-engines.sh"
export LANG="${LANG:-en_US.UTF-8}"
export LC_ALL="${LC_ALL:-en_US.UTF-8}"

if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
  export PUB_HOSTED_URL="${PUB_HOSTED_URL:-https://pub.dev}"
  export FLUTTER_STORAGE_BASE_URL="${FLUTTER_STORAGE_BASE_URL:-https://storage.googleapis.com}"
  # macos-14 的默认 Xcode 仍是 15.4（iOS 17.5 SDK）。选当前镜像里最新的 Xcode 26。
  xcode="$(python3 - <<'PY'
import glob, re
cands = glob.glob("/Applications/Xcode_26*.app") or glob.glob("/Applications/Xcode_*.app")
def key(path):
    m = re.search(r"Xcode_(\d+(?:\.\d+)*)", path)
    return tuple(int(x) for x in m.group(1).split(".")) if m else (0,)
print(max(cands, key=key) if cands else "")
PY
)"
  if [[ -n "${xcode:-}" ]]; then
    sudo xcode-select -s "$xcode"
  fi
  xcodebuild -version
  xcodebuild -showsdks | awk '/iphoneos/ { print }'
fi

kotv_ios_macho() {
  local bin="$1"
  [[ -f "$bin" && ! -L "$bin" ]] || return 1
  file -b "$bin" | grep -q 'Mach-O'
}

kotv_ios_framework_ok() {
  local app="$1" name="$2"
  local dest="$app/Frameworks/${name}.framework"
  [[ -d "$dest" && ! -L "$dest" ]] || return 1
  kotv_ios_macho "$dest/$name"
}

# 指向 App 外面的符号链接换成实体文件。包内的 Versions 链接保留。
kotv_ios_materialize_outside_links() {
  local app="$1"
  local link resolved
  while IFS= read -r link; do
    [[ -n "$link" ]] || continue
    resolved="$(python3 -c 'import os,sys; print(os.path.realpath(sys.argv[1]))' "$link" 2>/dev/null || true)"
    [[ -n "$resolved" && -e "$resolved" ]] || continue
    python3 -c 'import os,sys; root,path=os.path.realpath(sys.argv[1]),os.path.realpath(sys.argv[2]); raise SystemExit(0 if path==root or path.startswith(root+os.sep) else 1)' "$app" "$resolved" && continue
    rm -rf "$link"
    if [[ -d "$resolved" ]]; then
      cp -R "$resolved" "$link"
    else
      cp -L "$resolved" "$link"
    fi
  done < <(find "$app" -type l | awk '{ print length, $0 }' | sort -nr | cut -d' ' -f2-)
}

kotv_ios_copy_framework() {
  local src="$1" dest_parent="$2"
  local name dest
  name="$(basename "$src")"
  dest="$dest_parent/$name"
  rm -rf "$dest"
  mkdir -p "$dest_parent"
  cp -R "$src" "$dest"
}

# 只收 ios-arm64，避开模拟器切片。
kotv_ios_device_slices() {
  local root name parent
  for root in "$@"; do
    [[ -d "$root" ]] || continue
    while IFS= read -r fw; do
      [[ -n "$fw" ]] || continue
      parent="$(basename "$(dirname "$fw")")"
      [[ "$parent" == "ios-arm64" ]] || continue
      echo "$fw"
    done < <(find "$root" -type d -name '*.framework' 2>/dev/null || true)
  done
}

kotv_ios_built_framework() {
  local name="$1"
  shift
  local fw hit=""
  for root in "$@"; do
    [[ -d "$root" ]] || continue
    while IFS= read -r fw; do
      [[ -n "$fw" ]] || continue
      case "$fw" in
        *simulator*|*iphonesimulator*) continue ;;
        *iphoneos*) ;;
        *) continue ;;
      esac
      [[ -e "$fw/$name" ]] || continue
      if [[ "$fw" == *Release-iphoneos* ]]; then
        echo "$fw"
        return 0
      fi
      hit="$fw"
    done < <(find "$root" -type d -name "${name}.framework" 2>/dev/null || true)
  done
  [[ -n "$hit" ]] && echo "$hit"
}

kotv_ios_prepare_app() {
  local app="$1"
  local fw name built
  kotv_ios_materialize_outside_links "$app"
  local pub_cache="${PUB_CACHE:-${PUB_CACHE_DIR:-$HOME/.pub-cache}}"
  local -a slice_roots=(
    "${KOTV_MDK_POD_PATH:-}"
    "$ROOT/flutter/ios/.symlinks/plugins"
    "$ROOT/flutter/ios/Pods"
    "$pub_cache/hosted"
  )
  while IFS= read -r fw; do
    [[ -n "$fw" ]] || continue
    name="$(basename "$fw" .framework)"
    if kotv_ios_framework_ok "$app" "$name"; then
      continue
    fi
    echo "==> embed device framework $name"
    kotv_ios_copy_framework "$fw" "$app/Frameworks"
  done < <(kotv_ios_device_slices "${slice_roots[@]}")

  local -a built_roots=(
    "$ROOT/flutter/build/ios"
    "$HOME/Library/Developer/Xcode/DerivedData"
  )
  local -a built_names=(
    file_selector_ios
    fvp
    media_kit_libs_ios_video
    media_kit_video
    package_info_plus
    shared_preferences_foundation
    url_launcher_ios
    video_player_avfoundation
    volume_controller
    wakelock_plus
  )
  for name in "${built_names[@]}"; do
    if kotv_ios_framework_ok "$app" "$name"; then
      continue
    fi
    built="$(kotv_ios_built_framework "$name" "${built_roots[@]}" || true)"
    if [[ -z "$built" ]]; then
      echo "error: built iOS framework not found: $name" >&2
      return 1
    fi
    echo "==> embed built framework $name"
    kotv_ios_copy_framework "$built" "$app/Frameworks"
  done

  local -a required=(App Flutter mdk Mpv fvp)
  for name in "${required[@]}"; do
    if ! kotv_ios_framework_ok "$app" "$name"; then
      echo "error: $app is missing a real $name.framework" >&2
      return 1
    fi
  done
  echo "==> iOS app frameworks:"
  find "$app/Frameworks" -maxdepth 1 -type d -name '*.framework' -exec basename {} \; | sort
  kotv_ios_strip_unused_assets "$app"
  du -sh "$app" "$app/Frameworks"
}

# iOS 没有本地引擎。assets/engine 只剩占位说明；网页脚本在 iOS 上也不加载。
# 删掉文件后再清因此变空的目录，避免包里留下空文件夹。
kotv_ios_strip_unused_assets() {
  local app="$1"
  local assets="$app/Frameworks/App.framework/flutter_assets"
  [[ -d "$assets" ]] || return 0
  rm -rf \
    "$assets/assets/engine" \
    "$assets/packages/media_kit" \
    "$assets/packages/wakelock_plus"
  python3 - "$assets" <<'PY'
import os, sys
root = sys.argv[1]
for dirpath, dirnames, filenames in os.walk(root, topdown=False):
    if dirpath == root:
        continue
    if not os.listdir(dirpath):
        os.rmdir(dirpath)
        print("removed empty", os.path.relpath(dirpath, root), flush=True)
PY
}

kotv_ios_require_ipa_entry() {
  local ipa="$1" path="$2" min="$3"
  local size
  size="$(unzip -l "$ipa" | awk -v p="$path" '$4 == p && !seen { print $1; seen=1 }')"
  if [[ -z "${size:-}" || "$size" -lt "$min" ]]; then
    echo "error: IPA missing $path (size=${size:-0}, need >= $min)" >&2
    unzip -l "$ipa" | awk '$4 ~ /Frameworks/ { print }' >&2 || true
    return 1
  fi
}

main() {
  if ! command -v pod >/dev/null 2>&1; then
    brew install cocoapods
  fi
  pod --version
  # shellcheck source=kotv-fvp-deps.sh
  source "$ROOT/scripts/kotv-fvp-deps.sh"
  kotv_ensure_mdk_apple_pod
  export KOTV_MDK_POD_PATH

  flutter config --no-enable-swift-package-manager || true
  "$ROOT/scripts/flutter-pub-get.sh"

  cd "$ROOT/flutter"
  kotv_flutter_asset_engines_hide
  trap kotv_flutter_asset_engines_restore EXIT
  flutter build ipa --no-codesign
  local app="build/ios/archive/Runner.xcarchive/Products/Applications/Runner.app"
  [[ -d "$app" ]] || {
    echo "error: missing $app" >&2
    exit 1
  }
  kotv_ios_prepare_app "$ROOT/flutter/$app"

  local ver stage out
  ver="$(kotv_release_version "$ROOT/flutter/pubspec.yaml")"
  stage="$(mktemp -d)"
  mkdir -p "$stage/Payload" "$ROOT/dist"
  ditto "$ROOT/flutter/$app" "$stage/Payload/Runner.app"
  out="$ROOT/dist/KO影视-${ver}-ios.ipa"
  rm -f "$out"
  # -D：不把目录本身写进 zip。文件还在，空目录项不会再出现。
  (cd "$stage" && zip -r -y -D -q "$out" Payload)
  cp -f "$out" "$ROOT/KO影视-${ver}-ios.ipa"
  ls -lh "$out"
  kotv_ios_require_ipa_entry "$out" "Payload/Runner.app/Frameworks/App.framework/App" 5000000
  kotv_ios_require_ipa_entry "$out" "Payload/Runner.app/Frameworks/Flutter.framework/Flutter" 1000000
  kotv_ios_require_ipa_entry "$out" "Payload/Runner.app/Frameworks/mdk.framework/mdk" 1000000
  kotv_ios_require_ipa_entry "$out" "Payload/Runner.app/Frameworks/Mpv.framework/Mpv" 500000
  kotv_ios_require_ipa_entry "$out" "Payload/Runner.app/Frameworks/fvp.framework/fvp" 50000
  kotv_ios_require_ipa_entry "$out" "Payload/Runner.app/Frameworks/Avcodec.framework/Avcodec" 500000
  kotv_flutter_asset_engines_reject "$out"
  kotv_ios_reject_asset_clutter "$out"
}

kotv_ios_reject_asset_clutter() {
  local ipa="$1"
  python3 - "$ipa" <<'PY'
import sys, zipfile
ipa = sys.argv[1]
bad = []
with zipfile.ZipFile(ipa) as z:
    for info in z.infolist():
        name = info.filename
        if name.endswith("/") or info.is_dir():
            bad.append("dir " + name)
            continue
        if "flutter_assets/assets/engine/" in name:
            bad.append(name)
        elif name.endswith("/hls1.4.10.js") or name.endswith("/no_sleep.js"):
            bad.append(name)
if bad:
    print("\n".join(bad[:40]), file=sys.stderr)
    sys.exit(1)
print(f"ok: {ipa} has no engine dir, web assets, or empty directory entries")
PY
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
