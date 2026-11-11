#!/usr/bin/env bash
# 校验：Vulkan stub 仅在 assets；kotv_dl 在 jniLibs；勿再自带 libmpv 套件（Media3 AAR）。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ASSET="$ROOT/flutter/android/app/src/main/assets/mpv-libs"
JNI_ROOT="$ROOT/flutter/android/app/src/main/jniLibs"
fail=0

check_abi() {
  local abi="$1"
  local asset_dir="$ASSET/$abi"
  local jni="$JNI_ROOT/$abi"
  local vulkan_asset="$asset_dir/libvulkan.so"
  local vulkan_jni="$jni/libvulkan.so"
  local dl_jni="$jni/libkotv_dl.so"

  [[ -f "$vulkan_asset" ]] || { echo "ERROR: missing $vulkan_asset (API25 stub in assets)" >&2; fail=1; return; }
  [[ -f "$dl_jni" ]] || { echo "ERROR: missing $dl_jni" >&2; fail=1; return; }
  if [[ -f "$vulkan_jni" ]]; then
    echo "ERROR: $vulkan_jni must not exist (stub in jniLibs steals system Vulkan)" >&2
    fail=1
    return
  fi
  local dup
  for dup in libmpv.so libplayer.so libmvcodec.so libc++_shared.so libkotv_dl.so; do
    if [[ -f "$asset_dir/$dup" ]]; then
      echo "ERROR: duplicate $asset_dir/$dup (must not live in assets)" >&2
      fail=1
    fi
  done
  for dup in libmpv.so libplayer.so libmvcodec.so; do
    if [[ -f "$jni/$dup" ]]; then
      echo "ERROR: legacy $jni/$dup present (Media3 AAR supplies mpv natives)" >&2
      fail=1
    fi
  done
  echo "ok $abi: libvulkan stub in assets; kotv_dl in jniLibs; no legacy mpv suite"
}

echo "==> verify Android MPV helpers (Vulkan stub + kotv_dl; Media3 AAR for libmpv)"
check_abi arm64-v8a
check_abi armeabi-v7a
[[ "$fail" == 0 ]] || exit 1
echo "==> ok: Android MPV helper checks passed"
