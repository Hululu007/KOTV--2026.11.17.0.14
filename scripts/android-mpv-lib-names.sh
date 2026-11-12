#!/usr/bin/env bash
# Android MPV 相关 .so 名称（供 prepare / verify / gradle overlay 共用）。
# - media3-mpvplayer：libmpv / libplayer / 配套 libc++（含 from_chars）
# - media3-decoder-ffmpeg：整套 libav*（勿与 nextlib 的残缺 libav* pickFirst 混用）
# - 本仓：Vulkan stub（assets）+ kotv_dl（jniLibs）+ 上述抽出物（jniLibs，merge 后再盖一次）
# shellcheck disable=SC2034

KOTV_MEDIA3_MPV_VER="${KOTV_MEDIA3_MPV_VER:-1.11.0-alpha01-fongmi}"

# libmpv 依赖的 libc++ 符号；ass/fvp 旧 libc++ 会缺。
KOTV_ANDROID_LIBCXX_REQUIRED_SYM='_ZNSt6__ndk127__from_chars_floating_pointIfE'
# libavfilter → libavcodec：混用 nextlib avcodec 时会缺。
KOTV_ANDROID_LIBAV_REQUIRED_SYM='avpriv_elbg_free'

# 从 media3-decoder-ffmpeg 抽出、写入 jniLibs 并在 merge 后覆盖的 libav*。
KOTV_ANDROID_MPV_FFMPEG_JNI=(
  libavcodec.so
  libavfilter.so
  libavformat.so
  libavutil.so
  libavdevice.so
  libswresample.so
  libswscale.so
)

# merge 后必须用 jniLibs 覆盖的文件（mpvplayer libc++ + decoder-ffmpeg libav*）。
KOTV_ANDROID_MPV_MERGE_OVERLAY=(
  libc++_shared.so
  libavcodec.so
  libavfilter.so
  libavformat.so
  libavutil.so
  libavdevice.so
  libswresample.so
  libswscale.so
)

# 旧自研套件 / 曾手拷的 Media3 so：不得再出现在 jniLibs（与 AAR 冲突）。
# libc++ / libav* 不在此列——prepare 会从 AAR 写入。
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

# assets/mpv-libs 只保留 libvulkan stub。
KOTV_ANDROID_MPV_ASSET_FORBIDDEN=(
  libmpv.so
  libplayer.so
  libmvcodec.so
  libc++_shared.so
  libkotv_dl.so
)

# 旧自研编解码名：不得进最终 APK。
KOTV_ANDROID_MPV_APK_FORBIDDEN=(
  libmvcodec.so
  libmvutil.so
  libmwresample.so
  libmwscale.so
  libmvformat.so
  libmvfilter.so
  libmvdevice.so
)

# 最终 APK lib/<abi>/ 必须有。
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

kotv_android_nm_bin() {
  if command -v llvm-nm >/dev/null 2>&1; then
    echo llvm-nm
  elif command -v nm >/dev/null 2>&1; then
    echo nm
  else
    echo ""
  fi
}

# 若有 nm，检查 .so 是否导出给定符号子串。
kotv_android_so_has_sym() {
  local so="${1:-}"
  local sym="${2:-}"
  local nm_bin
  [[ -n "$so" && -f "$so" && -n "$sym" ]] || return 1
  nm_bin="$(kotv_android_nm_bin)"
  [[ -n "$nm_bin" ]] || return 0
  "$nm_bin" -D "$so" 2>/dev/null | grep -F "$sym" >/dev/null
}

kotv_android_libcxx_has_from_chars() {
  kotv_android_so_has_sym "${1:-}" "$KOTV_ANDROID_LIBCXX_REQUIRED_SYM"
}

kotv_android_libavcodec_is_complete() {
  kotv_android_so_has_sym "${1:-}" "$KOTV_ANDROID_LIBAV_REQUIRED_SYM"
}
