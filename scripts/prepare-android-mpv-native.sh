#!/usr/bin/env bash
# Media3 MpvPlayer：libmpv/libplayer/libc++ 来自 media3-mpvplayer AAR。
# 本脚本只准备 API25 Vulkan stub（assets）与 kotv_dl（FORCE_LOAD stub）。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=android-mpv-lib-names.sh
source "$ROOT/scripts/android-mpv-lib-names.sh"
chmod +x "$ROOT/scripts/build-android-libvulkan-stub.sh" \
  "$ROOT/scripts/build-android-kotv-dl.sh"

echo "==> prepare Android MPV helpers (Vulkan stub + kotv_dl; Media3 AAR supplies libmpv)"

for abi in arm64-v8a armeabi-v7a; do
  echo "==> native stubs: $abi"
  "$ROOT/scripts/build-android-libvulkan-stub.sh" "$abi"
  "$ROOT/scripts/build-android-kotv-dl.sh" "$abi"
done

strip_assets_keep_vulkan_stub() {
  local abi="$1"
  local base="$ROOT/flutter/android/app/src/main/assets/mpv-libs/$abi"
  local f name
  [[ -d "$base" ]] || return 0
  for f in "$base"/*; do
    [[ -f "$f" ]] || continue
    name="$(basename "$f")"
    if [[ "$name" == "libvulkan.so" ]]; then
      continue
    fi
    rm -f "$f"
  done
  echo "  ok $abi: assets keep libvulkan.so stub only"
}

# 旧自研套件勿再进 jniLibs（与 Media3 AAR 冲突）。
purge_legacy_mpv_jni() {
  local abi="$1"
  local jni="$ROOT/flutter/android/app/src/main/jniLibs/$abi"
  local lib
  [[ -d "$jni" ]] || return 0
  for lib in "${KOTV_ANDROID_MPV_LEGACY_JNI[@]}"; do
    if [[ -s "$jni/$lib" ]]; then
      rm -f "$jni/$lib"
      echo "  removed legacy $abi/$lib (Media3 AAR provides mpv)"
    fi
  done
  # stub 不得进 jniLibs。
  if [[ -s "$jni/libvulkan.so" ]]; then
    echo "WARN: removing jniLibs/$abi/libvulkan.so (must not ship stub in APK lib/)" >&2
    rm -f "$jni/libvulkan.so"
  fi
}

for abi in arm64-v8a armeabi-v7a; do
  strip_assets_keep_vulkan_stub "$abi"
  purge_legacy_mpv_jni "$abi"
done

verify_abi() {
  local abi="$1"
  local base="$ROOT/flutter/android/app/src/main/assets/mpv-libs/$abi"
  local jni="$ROOT/flutter/android/app/src/main/jniLibs/$abi"
  local missing=0
  if [[ ! -s "$base/libvulkan.so" ]]; then
    echo "ERROR: missing assets/mpv-libs/$abi/libvulkan.so (stub)" >&2
    missing=1
  fi
  if [[ ! -s "$jni/libkotv_dl.so" ]]; then
    echo "ERROR: missing jniLibs/$abi/libkotv_dl.so" >&2
    missing=1
  fi
  if [[ -s "$jni/libvulkan.so" ]]; then
    echo "ERROR: jniLibs/$abi/libvulkan.so must not exist" >&2
    missing=1
  fi
  local legacy
  for legacy in "${KOTV_ANDROID_MPV_LEGACY_JNI[@]}"; do
    if [[ -s "$jni/$legacy" ]]; then
      echo "ERROR: legacy $jni/$legacy still present (use Media3 AAR)" >&2
      missing=1
    fi
  done
  if [[ "$missing" != 0 ]]; then
    exit 1
  fi
  echo "  ok $abi: vulkan stub in assets; kotv_dl in jniLibs; no legacy mpv suite"
}

verify_abi arm64-v8a
verify_abi armeabi-v7a
chmod +x "$ROOT/scripts/verify-android-mpv-libs.sh"
"$ROOT/scripts/verify-android-mpv-libs.sh"
echo "==> Android MPV native prepare done"
