package com.bobo.kotv

/** 把百分比透明度乘到颜色原来的 alpha 上。透明底乘任何值仍透明。 */
internal fun applySubtitleOpacity(raw: String, opacityPct: Double): String {
  val s = raw.trim()
  if (s.isEmpty()) return s
  return try {
    val parsed = android.graphics.Color.parseColor(if (s.startsWith("#")) s else "#$s")
    val rgb = parsed and 0x00FFFFFF
    val origA = android.graphics.Color.alpha(parsed)
    val a = ((origA * opacityPct.coerceIn(0.0, 100.0) / 100.0).toInt()).coerceIn(0, 255)
    String.format("#%02X%06X", a, rgb)
  } catch (_: Throwable) {
    s
  }
}
