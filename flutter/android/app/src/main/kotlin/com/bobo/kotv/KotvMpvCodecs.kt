package com.bobo.kotv

/**
 * 安卓原生 MPV：图片/封面类编码名（与 Dart kotvIsImageMediaCodec 同一套）。
 */
internal object KotvMpvCodecs {

  fun imageCodec(codec: String?): Boolean {
    val raw = codec?.trim()?.lowercase().orEmpty()
    if (raw.isEmpty()) return false
    val base = raw.split('.', ' ', '/').firstOrNull { it.isNotEmpty() } ?: raw
    return when (base) {
      "png", "mjpeg", "mjpg", "jpeg", "jpg", "webp", "bmp", "gif", "tiff", "tif", "ppm", "pgm" -> true
      else -> raw.contains("png") || raw.startsWith("mjpeg") || raw.startsWith("mjpg")
    }
  }
}
