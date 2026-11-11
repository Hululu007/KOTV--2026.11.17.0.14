package com.bobo.kotv

/** Exo / Media3 Mpv 共用的解码与渲染归一。 */
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
}
