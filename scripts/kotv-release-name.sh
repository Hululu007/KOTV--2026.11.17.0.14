#!/usr/bin/env bash
# 统一发行名：KO影视-{version}-{arch}.{ext}
# 版本单一源：KOTV_VERSION 优先，否则 flutter/pubspec.yaml 的 version 行。
# 用法: kotv_release_version [pubspec路径]
kotv_release_version() {
  local pubspec="${1:-}"
  if [[ -n "${KOTV_VERSION:-}" ]]; then
    echo "${KOTV_VERSION#v}"
    return
  fi
  if [[ -z "$pubspec" ]]; then
    pubspec="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/flutter/pubspec.yaml"
  fi
  local v
  v="$(sed -nE 's/^version:[[:space:]]*([0-9]+\.[0-9]+\.[0-9]+).*/\1/p' "$pubspec" | head -1)"
  echo "${v:-0.0.0}"
}

# 编引擎时注入 update.CurrentVersion（与发行号同源）。可拼在已有 -ldflags 前。
# 用法: kotv_engine_version_ldflags [version]
kotv_engine_version_ldflags() {
  local ver="${1:-}"
  if [[ -z "$ver" ]]; then
    ver="$(kotv_release_version)"
  fi
  echo "-X github.com/bobo/KOTV/internal/update.CurrentVersion=${ver}"
}

# 核对 Go 默认值 / Flutter 回退常量 / Inno 默认与 pubspec 同源，避免关于/检查更新再脱节。
# 用法: kotv_check_version_literals [version]
kotv_check_version_literals() {
  local root ver go_ver dart_ver iss_ver
  root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  ver="${1:-$(kotv_release_version "$root/flutter/pubspec.yaml")}"
  go_ver="$(sed -nE 's/^var CurrentVersion = "([^"]+)".*/\1/p' "$root/internal/update/update.go" | head -1)"
  dart_ver="$(sed -nE "s/^const kotvClientVersion = '([^']+)'.*/\1/p" "$root/flutter/lib/api/kotv_engine_url.dart" | head -1)"
  iss_ver="$(sed -nE 's/^[[:space:]]*#define MyAppVersion "([^"]+)".*/\1/p' "$root/scripts/windows/kotv.iss" | head -1)"
  if [[ "$go_ver" != "$ver" || "$dart_ver" != "$ver" || "$iss_ver" != "$ver" ]]; then
    echo "ERROR: version literals drift (want $ver): CurrentVersion=$go_ver kotvClientVersion=$dart_ver MyAppVersion=$iss_ver" >&2
    return 1
  fi
}

# 平台 → 发行 arch 标签（与 Android aarch64/armv7 命名一致）
# 输入: arm64-v8a|armeabi-v7a|android-arm64|android-arm|macos-arm64|macos-x64|windows-x64|windows-arm64|linux-x64|...
kotv_release_arch() {
  case "$1" in
    arm64-v8a|android-arm64|macos-arm64|windows-arm64|linux-arm64|aarch64|arm64)
      echo "aarch64"
      ;;
    armeabi-v7a|android-arm|android-armv7|armv7)
      echo "armv7"
      ;;
    macos-x64|windows-x64|linux-x64|x86_64|amd64)
      echo "x86_64"
      ;;
    *)
      echo "$1"
      ;;
  esac
}
