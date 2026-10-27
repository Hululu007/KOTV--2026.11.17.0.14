#!/usr/bin/env bash
# CI：video_player_avfoundation 2.12.x 用到 macOS/iOS 26 SDK 的 AVFoundation 接口，须 Xcode 26。
# iOS / macOS 打包共用：优先 Xcode_26*，否则镜像里最新的 Xcode_*（不跟预览版默认值走）。
# shellcheck shell=bash

kotv_select_newest_xcode() {
  [[ "${GITHUB_ACTIONS:-}" == "true" ]] || return 0
  [[ "$(uname -s 2>/dev/null)" == "Darwin" ]] || return 0
  local xcode
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
}
