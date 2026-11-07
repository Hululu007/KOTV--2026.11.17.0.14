#!/usr/bin/env bash
# 用 nodejs-mobile 头文件 + NDK 编 libkotv_node.so，并拷贝同 ABI 的 libnode.so 进 flutter jniLibs。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CACHE="${KOTV_ANDROID_NODE_CACHE:-$ROOT/.runtime-cache/android-node}"
OUT_JNI="$ROOT/flutter/android/app/src/main/jniLibs"
NODE_MOBILE_VER="${NODE_MOBILE_VER:-v18.20.4}"
ZIP_NAME="nodejs-mobile-${NODE_MOBILE_VER}-android.zip"
ZIP_URL="https://github.com/nodejs-mobile/nodejs-mobile/releases/download/${NODE_MOBILE_VER}/${ZIP_NAME}"

ABIS=(arm64-v8a armeabi-v7a x86_64)

if [[ -z "${ANDROID_NDK_HOME:-${NDK_HOME:-}}" ]]; then
  # 常见本机 / CI 布局
  for cand in \
    "${ANDROID_HOME:-}/ndk-bundle" \
    "${ANDROID_SDK_ROOT:-}/ndk-bundle" \
    "$(ls -d "${ANDROID_HOME:-}/ndk/"* 2>/dev/null | sort -V | tail -1 || true)" \
    "$(ls -d "${ANDROID_SDK_ROOT:-}/ndk/"* 2>/dev/null | sort -V | tail -1 || true)" \
    "$HOME/Library/Android/sdk/ndk/"* ; do
    if [[ -n "$cand" && -d "$cand" ]]; then
      ANDROID_NDK_HOME="$cand"
      break
    fi
  done
fi
NDK="${ANDROID_NDK_HOME:-${NDK_HOME:-}}"
if [[ -z "$NDK" || ! -d "$NDK" ]]; then
  echo "ERROR: set ANDROID_NDK_HOME to build libkotv_node.so" >&2
  exit 1
fi

HOST_TAG=""
case "$(uname -s)" in
  Darwin) HOST_TAG=darwin-x86_64 ;;
  Linux) HOST_TAG=linux-x86_64 ;;
  *) echo "ERROR: unsupported host for NDK clang" >&2; exit 1 ;;
esac
CLANG_ROOT="$NDK/toolchains/llvm/prebuilt/$HOST_TAG"
if [[ ! -d "$CLANG_ROOT" ]]; then
  echo "ERROR: NDK clang missing at $CLANG_ROOT" >&2
  exit 1
fi

mkdir -p "$CACHE"
ZIP="$CACHE/$ZIP_NAME"
if [[ ! -s "$ZIP" ]]; then
  echo "[android-node] download $ZIP_URL"
  curl -fL --http1.1 --retry 3 -o "$ZIP.partial" "$ZIP_URL"
  mv -f "$ZIP.partial" "$ZIP"
fi
SRC="$CACHE/nodejs-mobile-$NODE_MOBILE_VER"
if [[ ! -d "$SRC/include/node" ]]; then
  rm -rf "$SRC"
  mkdir -p "$SRC"
  unzip -q "$ZIP" -d "$SRC"
fi
INC="$SRC/include/node"
SRC_CPP="$ROOT/scripts/android-node/kotv_node_main.cpp"
API=24

abi_triple() {
  case "$1" in
    arm64-v8a) echo aarch64-linux-android ;;
    armeabi-v7a) echo armv7a-linux-androideabi ;;
    x86_64) echo x86_64-linux-android ;;
    *) return 1 ;;
  esac
}

for abi in "${ABIS[@]}"; do
  if [[ ! -f "$SRC/bin/$abi/libnode.so" ]]; then
    echo "[android-node] skip $abi (no libnode.so in release)"
    continue
  fi
  triple="$(abi_triple "$abi")"
  CXX="$CLANG_ROOT/bin/${triple}${API}-clang++"
  if [[ ! -x "$CXX" ]]; then
    echo "ERROR: missing $CXX" >&2
    exit 1
  fi
  out_dir="$OUT_JNI/$abi"
  mkdir -p "$out_dir"
  out="$out_dir/libkotv_node.so"
  echo "[android-node] build $abi → $out"
  # 可执行 PIE；与同目录 libnode.so 一并打进 APK，动态链接器按 nativeLibraryDir 解析。
  "$CXX" -std=c++17 -O2 -fPIC -pie \
    -I"$INC" \
    "$SRC_CPP" \
    -L"$SRC/bin/$abi" -lnode \
    -llog -ldl -lm \
    -Wl,--no-undefined \
    -o "$out"
  chmod +x "$out"
  cp -f "$SRC/bin/$abi/libnode.so" "$out_dir/libnode.so"
done

echo "[android-node] done. libkotv_node.so + libnode.so → $OUT_JNI"
