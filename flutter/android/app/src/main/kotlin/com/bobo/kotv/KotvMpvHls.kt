package com.bobo.kotv

import android.net.Uri

/**
 * 安卓原生 MPV：HLS URL 强信号 / 网络址 / lavf 打开失败提示 / 图片轨编码名。
 * URL 规则见 Go `model.URLIsHls`；引擎会正文嗅探写准 format，本处只作快路径与失败再开。
 */
internal object KotvMpvHls {

  /** 地址是否像 HLS（仅强信号）。引擎 `/proxy/play?id=` 无 m3u8 字样时不算。 */
  fun urlLooksLikeHls(url: String): Boolean {
    val raw = url.trim()
    if (raw.isEmpty()) return false
    val uri = Uri.parse(raw)
    val scheme = uri.scheme?.lowercase().orEmpty()
    if (scheme == "rtsp" || scheme == "rtspt") return false
    if (scheme == "data") {
      val body = if (raw.length > 5) raw.substring(5) else ""
      val mime = body.substringBefore(',').lowercase().trim()
      return mime.contains("mpegurl") || mime.contains("m3u8")
    }
    val lower = raw.lowercase()
    if (lower.contains("=m3u8")) return true
    if (lower.contains("=mpd")) return false
    for (seg in uri.pathSegments) {
      if (seg.isEmpty()) continue
      if (seg.lowercase().contains("m3u8")) return true
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

  /** 静止图/封面类编码名（与 Dart kotvIsImageMediaCodec 同一套）。 */
  fun imageCodec(codec: String?): Boolean {
    val raw = codec?.trim()?.lowercase().orEmpty()
    if (raw.isEmpty) return false
    val base = raw.split('.', ' ', '/').firstOrNull { it.isNotEmpty() } ?: raw
    return when (base) {
      "png", "mjpeg", "mjpg", "jpeg", "jpg", "webp", "bmp", "gif", "tiff", "tif", "ppm", "pgm" -> true
      else -> raw.contains("png") || raw.startsWith("mjpeg") || raw.startsWith("mjpg")
    }
  }
}
