#!/usr/bin/env bash
# 校验 APK：Media3 mpvplayer 提供 libmpv/libplayer；Vulkan stub 仅在 assets；kotv_dl 在 lib/。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=android-mpv-lib-names.sh
source "$ROOT/scripts/android-mpv-lib-names.sh"
APK="${1:-}"
if [[ -z "$APK" || ! -f "$APK" ]]; then
  echo "usage: $0 <path-to.apk>" >&2
  exit 1
fi
listing="$(unzip -l "$APK" | awk 'NR>3 {print $4}')"
abis="$(printf '%s\n' "$listing" | awk -F/ '/^lib\//{print $2}' | sort -u)"
if [[ -z "$abis" ]]; then
  echo "ERROR: no lib/<abi>/ entries in APK" >&2
  exit 1
fi

need=()
while IFS= read -r abi; do
  [[ -n "$abi" ]] || continue
  case "$abi" in
    arm64-v8a|armeabi-v7a) ;;
    *) continue ;;
  esac
  need+=("assets/mpv-libs/${abi}/libvulkan.so")
  local_so=
  for local_so in "${KOTV_ANDROID_MPV_APK_REQUIRED[@]}"; do
    need+=("lib/${abi}/${local_so}")
  done
done <<<"$abis"

if [[ "${#need[@]}" -eq 0 ]]; then
  echo "ERROR: APK has no arm64-v8a/armeabi-v7a libs" >&2
  exit 1
fi

missing=0
for p in "${need[@]}"; do
  if ! grep -qxF "$p" <<<"$listing"; then
    echo "MISSING in APK: $p" >&2
    missing=1
  fi
done
if [[ "$missing" != 0 ]]; then
  exit 1
fi

foreign="$(printf '%s\n' "$listing" | awk -F/ -v have="$(printf '%s' "$abis" | tr '\n' ' ')" '
  $1=="assets" && $2=="mpv-libs" && $3!="" {
    abi=$3
    if (index(" " have " ", " " abi " ")==0) print
  }
')"
if [[ -n "$foreign" ]]; then
  echo "ERROR: APK contains mpv-libs for an ABI that is not in lib/:" >&2
  printf '%s\n' "$foreign" >&2
  exit 1
fi

if grep -qxF "assets/libmitv.so" <<<"$listing"; then
  if ! grep -qxF "armeabi-v7a" <<<"$abis"; then
    echo "ERROR: assets/libmitv.so is 32-bit and must not ship in an APK without armeabi-v7a" >&2
    exit 1
  fi
elif grep -qxF "armeabi-v7a" <<<"$abis"; then
  echo "ERROR: armeabi-v7a APK missing assets/libmitv.so" >&2
  exit 1
fi

while IFS= read -r abi; do
  [[ -n "$abi" ]] || continue
  case "$abi" in arm64-v8a|armeabi-v7a) ;; *) continue ;; esac
  # stub 不得出现在 lib/<abi>/。
  bad="lib/${abi}/libvulkan.so"
  if grep -qxF "$bad" <<<"$listing"; then
    echo "ERROR: APK must not contain $bad (stub steals system Vulkan)" >&2
    exit 1
  fi
  # assets 不得再塞套件 / helpers。
  for dup in "${KOTV_ANDROID_MPV_ASSET_FORBIDDEN[@]}"; do
    p="assets/mpv-libs/${abi}/${dup}"
    if grep -qxF "$p" <<<"$listing"; then
      echo "ERROR: APK must not contain $p (duplicate of lib/${abi}/ or legacy)" >&2
      exit 1
    fi
  done
  for dup in "${KOTV_ANDROID_MPV_APK_FORBIDDEN[@]}"; do
    p="lib/${abi}/${dup}"
    if grep -qxF "$p" <<<"$listing"; then
      echo "ERROR: APK must not contain $p (legacy suite)" >&2
      exit 1
    fi
  done
  for dup in "${KOTV_ANDROID_FVP_APK_FORBIDDEN[@]}"; do
    p="lib/${abi}/${dup}"
    if grep -qxF "$p" <<<"$listing"; then
      echo "ERROR: APK must not contain $p (Android does not ship FVP/mdk)" >&2
      exit 1
    fi
  done
done <<<"$abis"

# libc++ / libavcodec 必须是配套完整版（否则 libmpv dlopen 失败）。
while IFS= read -r abi; do
  [[ -n "$abi" ]] || continue
  case "$abi" in arm64-v8a|armeabi-v7a) ;; *) continue ;; esac
  tmp="$(mktemp)"
  unzip -p "$APK" "lib/${abi}/libc++_shared.so" >"$tmp"
  if ! kotv_android_libcxx_has_from_chars "$tmp"; then
    rm -f "$tmp"
    echo "ERROR: APK lib/${abi}/libc++_shared.so lacks ${KOTV_ANDROID_LIBCXX_REQUIRED_SYM}" >&2
    exit 1
  fi
  unzip -p "$APK" "lib/${abi}/libavcodec.so" >"$tmp"
  if ! kotv_android_libavcodec_is_complete "$tmp"; then
    rm -f "$tmp"
    echo "ERROR: APK lib/${abi}/libavcodec.so lacks ${KOTV_ANDROID_LIBAV_REQUIRED_SYM} (mixed with nextlib?)" >&2
    exit 1
  fi
  rm -f "$tmp"
done <<<"$abis"

echo "ok: APK Media3 MPV + libav* + mpvplayer libc++ in lib/; Vulkan stub in assets only (abis: $(echo "$abis" | tr '\n' ' '))"
unzip -l "$APK" | awk '/mpv-libs|libvulkan|libkotv_dl|libmpv\.so|libplayer\.so|libc\+\+|libav|libsw/ {print $1, $4}' | head -40
