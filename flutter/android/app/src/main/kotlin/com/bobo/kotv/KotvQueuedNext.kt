package com.bobo.kotv

/** Exo / 安卓原生 MPV 无缝排队共用：下一集元数据。 */
data class KotvQueuedNext(
  val id: String,
  val url: String,
  val headers: Map<String, String>,
  val mime: String?,
  val subs: List<Map<String, Any?>>,
  val startAtMs: Long = 0L,
)
