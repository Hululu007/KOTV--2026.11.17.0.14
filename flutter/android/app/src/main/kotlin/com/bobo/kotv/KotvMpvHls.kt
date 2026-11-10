package com.bobo.kotv

import android.net.Uri

/**
 * 安卓原生 MPV：HLS 地址 / 网络址 / lavf 打开失败提示。
 * 规则与 `mpv_hls_timeline.dart` 同一套；原生层不能调 Dart，故在此留副本。
 * 供片尾短 EOF、LOADING_FAILED 时决定是否强制 demuxer-lavf-format=hls 再开。
 */
internal object KotvMpvHls {

  /** 只看地址本身；引擎 `/proxy/play?id=` 无 m3u8 字样时不算 HLS。 */
  fun urlLooksLikeHls(url: String): Boolean {
    val raw = url.trim()
    if (raw.isEmpty()) return false
    val uri = Uri.parse(raw)
    val scheme = uri.scheme?.lowercase().orEmpty()
    if (scheme == "rtsp" || scheme == "rtspt") return false
    if (scheme == "data") {
      val body = if (raw.length > 5) raw.substring(5) else ""
      return !body.startsWith("application/dash+xml")
    }
    val lower = raw.lowercase()
    if (lower.contains("=m3u8")) return true
    if (lower.contains("=mpd")) return false
    for (seg in uri.pathSegments) {
      if (seg.isEmpty()) continue
      val s = seg.lowercase()
      if (s == "m3u8" || s.contains("m3u8")) return true
      val dot = s.lastIndexOf('.')
      if (dot >= 0 && dot < s.length - 1) {
        val ext = s.substring(dot + 1)
        if (ext == "m3u8" || ext == "php") return true
      }
    }
    return false
  }

  /**
   * end-file 错误文案是否像「主文件 lavf 打开失败 / 未识别 m3u8」。
   * 无 log prefix，无法排除 vd/ad 解码器报错（桌面 media_kit 侧有 prefix）。
   */
  fun openFailedHint(errorText: String?): Boolean {
    val t = errorText?.lowercase().orEmpty()
    return t.contains("avformat_open_input() failed") || t.contains("not detecting m3u8/hls")
  }

  fun networkPlayUrl(url: String): Boolean {
    val scheme = Uri.parse(url.trim()).scheme?.lowercase().orEmpty()
    return scheme == "http" ||
      scheme == "https" ||
      scheme == "rtmp" ||
      scheme == "rtsp" ||
      scheme == "rtspt" ||
      scheme == "proxy"
  }
}
