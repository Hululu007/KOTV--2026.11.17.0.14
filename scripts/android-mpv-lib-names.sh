#!/usr/bin/env bash
# Android MPV 相关 .so 名称（供 prepare / verify 共用）。
# - media3-mpvplayer：libmpv / libplayer / libc++
# - media3-decoder-ffmpeg：libmpv DT_NEEDED 的 libav*（缺了会 CREATE_FAILED / dlopen 失败）
# - 本仓：Vulkan stub（assets）+ kotv_dl（jniLibs）
# shellcheck disable=SC2034

# 旧自研套件 / 曾手拷的 Media3 so：不得再出现在 jniLibs（与 AAR 冲突）。
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

# 旧自研编解码名：不得进最终 APK（libmpv/libplayer 由 Media3 AAR 正常打进 lib/）。
KOTV_ANDROID_MPV_APK_FORBIDDEN=(
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

# 最终 APK lib/<abi>/ 必须有（libmpv DT_NEEDED + Media3 mpvplayer + kotv_dl）。
KOTV_ANDROID_MPV_APK_REQUIRED=(
  libmpv.so
  libplayer.so
  libkotv_dl.so
  libc++_shared.so
  libavcodec.so
  libavfilter.so
  libavformat.so
  libavutil.so
  libavdevice.so
  libswresample.so
  libswscale.so
)
