#!/usr/bin/env bash
# Android MPV 相关 .so 名称（Media3 mpvplayer AAR 提供 libmpv/libplayer；本仓只自备 stub + kotv_dl）。
# 供 prepare / verify 共用，避免三处各抄一份。
# shellcheck disable=SC2034

# 旧自研套件：不得再出现在 jniLibs（会与 Media3 AAR 冲突）。
KOTV_ANDROID_MPV_LEGACY_JNI=(
  libmpv.so
  libplayer.so
  libmvcodec.so
  libmvutil.so
  libmwresample.so
  libmwscale.so
  libmvformat.so
  libmvfilter.so
  libmvdevice.so
)

# assets/mpv-libs 只保留 libvulkan stub；下列不得再出现在 assets。
KOTV_ANDROID_MPV_ASSET_FORBIDDEN=(
  libmpv.so
  libplayer.so
  libmvcodec.so
  libc++_shared.so
  libkotv_dl.so
)

# 最终 APK lib/<abi>/ 必须有（Media3 AAR + 本仓 helpers）。
KOTV_ANDROID_MPV_APK_REQUIRED=(
  libmpv.so
  libplayer.so
  libkotv_dl.so
  libc++_shared.so
)
