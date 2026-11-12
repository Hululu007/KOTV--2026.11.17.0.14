#!/usr/bin/env bash
# 校验：Vulkan stub 仅在 assets；kotv_dl + mpvplayer libc++ + decoder-ffmpeg libav* 在 jniLibs。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=android-mpv-lib-names.sh
source "$ROOT/scripts/android-mpv-lib-names.sh"
ASSET="$ROOT/flutter/android/app/src/main/assets/mpv-libs"
JNI_ROOT="$ROOT/flutter/android/app/src/main/jniLibs"
fail=0

check_abi() {
  local abi="$1"
  local asset_dir="$ASSET/$abi"
  local jni="$JNI_ROOT/$abi"
  local so dup

  [[ -f "$asset_dir/libvulkan.so" ]] || { echo "ERROR: missing $asset_dir/libvulkan.so" >&2; fail=1; return; }
  [[ -f "$jni/libkotv_dl.so" ]] || { echo "ERROR: missing $jni/libkotv_dl.so" >&2; fail=1; return; }
  [[ -s "$jni/libc++_shared.so" ]] || { echo "ERROR: missing $jni/libc++_shared.so" >&2; fail=1; return; }
  if ! kotv_android_libcxx_has_from_chars "$jni/libc++_shared.so"; then
    echo "ERROR: $jni/libc++_shared.so lacks ${KOTV_ANDROID_LIBCXX_REQUIRED_SYM}" >&2
    fail=1
    return
  fi
  for so in "${KOTV_ANDROID_MPV_FFMPEG_JNI[@]}"; do
    if [[ ! -s "$jni/$so" ]]; then
      echo "ERROR: missing $jni/$so" >&2
      fail=1
    fi
  done
  if [[ -s "$jni/libavcodec.so" ]] && ! kotv_android_libavcodec_is_complete "$jni/libavcodec.so"; then
    echo "ERROR: $jni/libavcodec.so lacks ${KOTV_ANDROID_LIBAV_REQUIRED_SYM}" >&2
    fail=1
  fi
  if [[ -f "$jni/libvulkan.so" ]]; then
    echo "ERROR: $jni/libvulkan.so must not exist" >&2
    fail=1
  fi
  for dup in "${KOTV_ANDROID_MPV_ASSET_FORBIDDEN[@]}"; do
    if [[ -f "$asset_dir/$dup" ]]; then
      echo "ERROR: duplicate $asset_dir/$dup" >&2
      fail=1
    fi
  done
  for dup in "${KOTV_ANDROID_MPV_LEGACY_JNI[@]}"; do
    if [[ -f "$jni/$dup" ]]; then
      echo "ERROR: legacy $jni/$dup present" >&2
      fail=1
    fi
  done
  echo "ok $abi: stub + kotv_dl + mpvplayer libc++ + decoder-ffmpeg libav*"
}

echo "==> verify Android MPV helpers"
check_abi arm64-v8a
check_abi armeabi-v7a
[[ "$fail" == 0 ]] || exit 1
echo "==> ok: Android MPV helper checks passed"
