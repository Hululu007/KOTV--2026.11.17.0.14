package com.bobo.kotv

import android.app.ActivityManager
import android.content.Context
import kotlin.math.roundToLong

/**
 * 与 Dart KotvBufferBudget.fromDevice 同一条比例：
 * 约 15% 可用内存，且不超过总内存 5%。不设固定字节上下限。
 * 读不到总内存时，桌面按 8GiB、其它按 3GiB 估算后再取 5%。
 * Exo 的 targetBufferBytes 用这份结果。
 */
object KotvMemBudget {
  private const val GIB = 1024L * 1024L * 1024L

  fun bytes(ctx: Context?, desktop: Boolean = false): Int {
    var total = 0L
    var avail = 0L
    if (ctx != null) {
      try {
        val am = ctx.getSystemService(Context.ACTIVITY_SERVICE) as ActivityManager
        val mi = ActivityManager.MemoryInfo()
        am.getMemoryInfo(mi)
        total = mi.totalMem
        avail = mi.availMem
      } catch (_: Throwable) {
      }
    }
    if (total <= 0L) {
      total = if (desktop) 8L * GIB else 3L * GIB
    }
    var budget = (total * 0.05).roundToLong()
    if (avail > 0L) {
      val byAvail = (avail * 0.15).roundToLong()
      if (byAvail in 1 until budget) budget = byAvail
    }
    if (budget < 1L) budget = 1L
    return budget.coerceAtMost(Int.MAX_VALUE.toLong()).toInt()
  }
}
