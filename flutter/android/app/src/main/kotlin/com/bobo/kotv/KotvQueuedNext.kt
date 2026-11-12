package com.bobo.kotv

/** Exo / Media3 Mpv 无缝排队共用：下一集元数据。 */
data class KotvQueuedNext(
  val id: String,
  val url: String,
  val headers: Map<String, String>,
  val mime: String?,
  val subs: List<Map<String, Any?>>,
  val startAtMs: Long = 0L,
)
