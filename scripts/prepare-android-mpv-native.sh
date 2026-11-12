#!/usr/bin/env bash
# Media3 MpvPlayer 辅助准备：
# - assets：API25 Vulkan stub
# - jniLibs：kotv_dl；从 media3-mpvplayer 抽 libc++；从 media3-decoder-ffmpeg 抽整套 libav*
#   （pickFirst 会误选 ass 旧 libc++ / nextlib 残缺 libav* → CREATE_FAILED）
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=android-mpv-lib-names.sh
source "$ROOT/scripts/android-mpv-lib-names.sh"
chmod +x "$ROOT/scripts/build-android-libvulkan-stub.sh" \
  "$ROOT/scripts/build-android-kotv-dl.sh"

MAVEN="$ROOT/flutter/android/third_party/maven-webhtv"
MPV_AAR="$MAVEN/androidx/media3/media3-mpvplayer/${KOTV_MEDIA3_MPV_VER}/media3-mpvplayer-${KOTV_MEDIA3_MPV_VER}.aar"
FFMPEG_AAR="$MAVEN/androidx/media3/media3-decoder-ffmpeg/${KOTV_MEDIA3_MPV_VER}/media3-decoder-ffmpeg-${KOTV_MEDIA3_MPV_VER}.aar"

echo "==> prepare Android MPV helpers (Vulkan stub + kotv_dl + mpvplayer libc++ + decoder-ffmpeg libav*)"

for aar in "$MPV_AAR" "$FFMPEG_AAR"; do
  if [[ ! -f "$aar" ]]; then
    echo "ERROR: missing $aar (run scripts/fetch-android-exo-av3a.sh / build-fongmi-media3.sh)" >&2
    exit 1
  fi
done

extract_aar_so() {
  local aar="$1"
  local abi="$2"
  local so="$3"
  local dest="$4"
  local entry="jni/${abi}/${so}"
  if ! unzip -l "$aar" | awk '{print $4}' | grep -qxF "$entry"; then
    echo "ERROR: $aar missing $entry" >&2
    exit 1
  fi
  mkdir -p "$(dirname "$dest")"
  unzip -p "$aar" "$entry" >"$dest"
  if [[ ! -s "$dest" ]]; then
    echo "ERROR: failed to extract $entry → $dest" >&2
    exit 1
  fi
}

install_mpvplayer_libcxx() {
  local abi="$1"
  local dest="$ROOT/flutter/android/app/src/main/jniLibs/$abi/libc++_shared.so"
  extract_aar_so "$MPV_AAR" "$abi" "libc++_shared.so" "$dest"
  if ! kotv_android_libcxx_has_from_chars "$dest"; then
    echo "ERROR: extracted $dest lacks ${KOTV_ANDROID_LIBCXX_REQUIRED_SYM}" >&2
    exit 1
  fi
  echo "  ok $abi: jniLibs/libc++_shared.so from media3-mpvplayer ($(wc -c <"$dest" | tr -d ' ') bytes)"
}

install_decoder_ffmpeg_libav() {
  local abi="$1"
  local jni="$ROOT/flutter/android/app/src/main/jniLibs/$abi"
  local so dest
  mkdir -p "$jni"
  for so in "${KOTV_ANDROID_MPV_FFMPEG_JNI[@]}"; do
    dest="$jni/$so"
    extract_aar_so "$FFMPEG_AAR" "$abi" "$so" "$dest"
  done
  if ! kotv_android_libavcodec_is_complete "$jni/libavcodec.so"; then
    echo "ERROR: $jni/libavcodec.so lacks ${KOTV_ANDROID_LIBAV_REQUIRED_SYM}" >&2
    exit 1
  fi
  echo "  ok $abi: jniLibs libav* from media3-decoder-ffmpeg ($(wc -c <"$jni/libavcodec.so" | tr -d ' ') bytes avcodec)"
}

for abi in arm64-v8a armeabi-v7a; do
  echo "==> native stubs: $abi"
  "$ROOT/scripts/build-android-libvulkan-stub.sh" "$abi"
  "$ROOT/scripts/build-android-kotv-dl.sh" "$abi"
  install_mpvplayer_libcxx "$abi"
  install_decoder_ffmpeg_libav "$abi"
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
  local so
  if [[ ! -s "$base/libvulkan.so" ]]; then
    echo "ERROR: missing assets/mpv-libs/$abi/libvulkan.so (stub)" >&2
    missing=1
  fi
  if [[ ! -s "$jni/libkotv_dl.so" ]]; then
    echo "ERROR: missing jniLibs/$abi/libkotv_dl.so" >&2
    missing=1
  fi
  if [[ ! -s "$jni/libc++_shared.so" ]]; then
    echo "ERROR: missing jniLibs/$abi/libc++_shared.so" >&2
    missing=1
  elif ! kotv_android_libcxx_has_from_chars "$jni/libc++_shared.so"; then
    echo "ERROR: jniLibs/$abi/libc++_shared.so lacks ${KOTV_ANDROID_LIBCXX_REQUIRED_SYM}" >&2
    missing=1
  fi
  for so in "${KOTV_ANDROID_MPV_FFMPEG_JNI[@]}"; do
    if [[ ! -s "$jni/$so" ]]; then
      echo "ERROR: missing jniLibs/$abi/$so" >&2
      missing=1
    fi
  done
  if [[ -s "$jni/libavcodec.so" ]] && ! kotv_android_libavcodec_is_complete "$jni/libavcodec.so"; then
    echo "ERROR: jniLibs/$abi/libavcodec.so lacks ${KOTV_ANDROID_LIBAV_REQUIRED_SYM}" >&2
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
  echo "  ok $abi: stub + kotv_dl + mpvplayer libc++ + decoder-ffmpeg libav*; no legacy suite"
}

verify_abi arm64-v8a
verify_abi armeabi-v7a
chmod +x "$ROOT/scripts/verify-android-mpv-libs.sh"
"$ROOT/scripts/verify-android-mpv-libs.sh"
echo "==> Android MPV native prepare done"
