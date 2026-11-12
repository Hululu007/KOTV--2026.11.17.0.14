package com.bobo.kotv

/** Exo / 安卓原生 MPV 共用的解码、渲染与请求头归一。 */
object KotvPlayerNormalize {
  fun decodeModeLabel(raw: String): String =
    when (raw.trim().lowercase()) {
      "soft", "software", "sw", "no" -> "soft"
      "hard", "hardware", "hw", "mediacodec" -> "hard"
      else -> "auto"
    }

  fun isTextureRender(raw: String): Boolean =
    when (raw.trim().lowercase()) {
      "texture", "textureview", "1" -> true
      else -> false
    }

  fun resolveRenderTexture(requested: String): Boolean =
    if (requested.isNotEmpty()) isTextureRender(requested) else false

  /** 去空 key/value 并 trim；供 open / queueNext 比较 headers。 */
  fun normalizeHeaders(raw: Map<String, String>?): Map<String, String> {
    if (raw.isNullOrEmpty()) return emptyMap()
    val out = LinkedHashMap<String, String>()
    for ((k0, v0) in raw) {
      val k = k0.trim()
      val v = v0.trim()
      if (k.isEmpty() || v.isEmpty()) continue
      out[k] = v
    }
    return out
  }

  fun clampPlayRate(raw: Double): Double = raw.coerceIn(0.25, 4.0)

  fun clampSubtitleScale(raw: Double): Double = raw.coerceIn(0.5, 2.0)
}
