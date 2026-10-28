package com.bobo.kotv

import android.content.Context
import android.hardware.display.DisplayManager
import android.os.Build
import android.view.Display

/** 默认显示器的 HDR 能力：决定杜比视界能否直出，也给播放信息显示。 */
internal object KotvDisplayHdr {
  /** Display.HdrCapabilities.HDR_TYPE_*；API 34 起按当前显示模式取。 */
  fun supportedTypes(ctx: Context): IntArray {
    return try {
      val display = ctx.getSystemService(DisplayManager::class.java)
        ?.getDisplay(Display.DEFAULT_DISPLAY)
        ?: return IntArray(0)
      if (Build.VERSION.SDK_INT >= 34) {
        display.mode.supportedHdrTypes
      } else {
        @Suppress("DEPRECATION")
        display.hdrCapabilities?.supportedHdrTypes ?: IntArray(0)
      }
    } catch (_: Throwable) {
      IntArray(0)
    }
  }

  fun supportsDolbyVision(ctx: Context): Boolean =
    Display.HdrCapabilities.HDR_TYPE_DOLBY_VISION in supportedTypes(ctx)

  /** 例：「杜比视界/HDR10/HLG」；都不支持时为「SDR」。 */
  fun label(ctx: Context): String {
    val names = supportedTypes(ctx).asList().mapNotNull {
      when (it) {
        Display.HdrCapabilities.HDR_TYPE_DOLBY_VISION -> "杜比视界"
        Display.HdrCapabilities.HDR_TYPE_HDR10 -> "HDR10"
        Display.HdrCapabilities.HDR_TYPE_HLG -> "HLG"
        4 -> "HDR10+" // HDR_TYPE_HDR10_PLUS（API 29）
        else -> null
      }
    }.distinct()
    return if (names.isEmpty()) "SDR" else names.joinToString("/")
  }
}
