package com.bobo.kotv

import android.content.Context
import android.os.Environment
import android.util.Log
import java.io.File

/**
 * 安卓侧可再生缓存：与 [com.github.catvod.utils.Path] 落盘一致。
 *
 * Go [cacheclean] 只扫引擎数据根（filesDir/KOTV）；爬虫 jar/js/py、exo/mpv、
 * 以及外置存储 `/TV/s_c_*` 必须由宿主清。
 */
object KotvCacheClean {
  private const val TAG = "KotvCacheClean"

  fun clear(
    ctx: Context,
    script: Boolean,
    jar: Boolean,
    other: Boolean,
    all: Boolean,
  ) {
    val cache = ctx.cacheDir
    if (script) {
      wipe(File(cache, "js"))
      wipe(File(cache, "py"))
    }
    if (jar) {
      wipe(File(cache, "jar"))
      wipe(File(ctx.codeCacheDir, "kotv_site_jars"))
    }
    if (other) {
      wipe(File(cache, "epg"))
      wipe(File(cache, "jpa"))
      wipe(File(cache, "mpv"))
      wipe(File(cache, "proxy"))
      try {
        KotvExoCache.clear(ctx)
      } catch (t: Throwable) {
        Log.w(TAG, "exo clear failed", t)
      }
    }
    // 与 Go sweep 一致：仅「全部」删历史 TV/s_c_*；保留 /TV/ 根凭证与 s_u_<用户名>。
    if (all) {
      removeLegacyClientTVDirs()
    }
  }

  private fun wipe(dir: File) {
    if (!dir.exists()) return
    try {
      if (!dir.deleteRecursively()) {
        Log.w(TAG, "wipe incomplete: $dir")
      }
    } catch (t: Throwable) {
      Log.w(TAG, "wipe failed: $dir", t)
    }
  }

  /** 外置 `/TV/s_c_*`；本机凭证在 `/TV/.name`，远端在 `/TV/s_u_<用户名>`。 */
  private fun removeLegacyClientTVDirs() {
    val tv = File(Environment.getExternalStorageDirectory(), "TV")
    if (!tv.isDirectory) return
    val kids = tv.listFiles() ?: return
    for (f in kids) {
      if (!f.isDirectory) continue
      if (!f.name.lowercase().startsWith("s_c_")) continue
      wipe(f)
    }
  }
}
