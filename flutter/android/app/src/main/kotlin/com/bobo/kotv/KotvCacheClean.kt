package com.bobo.kotv

import android.content.Context
import android.os.Environment
import android.util.Log
import java.io.File

/**
 * 安卓侧可再生缓存：与 [com.github.catvod.utils.Path] / Go CacheRoot（KOTV_CACHE_DIR）同根。
 *
 * 爬虫包体在 cacheDir/{jar,js,py,node}；可变态在 cacheDir/run/…。
 * Go cacheclean 主要扫引擎数据根（filesDir/KOTV）；宿主负责 cacheDir 与外置 `/TV/s_c_*`。
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
      wipe(File(cache, "node"))
      wipeRuntimeSubs(cache, "js-local", "node", "py")
    }
    if (jar) {
      wipe(File(cache, "jar"))
      wipe(File(ctx.codeCacheDir, "kotv_site_jars"))
      wipeRuntimeSubs(cache, "jvm")
    }
    if (other) {
      wipe(File(cache, "epg"))
      wipe(File(cache, "jpa"))
      wipe(File(cache, "mpv"))
      wipe(File(cache, "mpv-cache"))
      wipe(File(cache, "proxy"))
      try {
        KotvExoCache.clear(ctx)
      } catch (t: Throwable) {
        Log.w(TAG, "exo clear failed", t)
      }
    }
    // 与 Go paths.IsLegacyTVDir 一致：全部时删 s_c_* / 旧 s_u_<uuid>；保留 s_u_<用户名>。
    if (all) {
      removeLegacyTVDirs()
    }
  }

  private fun wipeRuntimeSubs(cache: File, vararg names: String) {
    val run = File(cache, "run")
    val users = run.listFiles() ?: return
    for (user in users) {
      if (!user.isDirectory) continue
      for (name in names) wipe(File(user, name))
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

  /** 外置 `/TV/`：本机 `.name`、远端 `s_u_<用户名>` 保留。 */
  private fun removeLegacyTVDirs() {
    val tv = File(Environment.getExternalStorageDirectory(), "TV")
    if (!tv.isDirectory) return
    val kids = tv.listFiles() ?: return
    for (f in kids) {
      if (!f.isDirectory) continue
      if (!isLegacyTVDir(f.name)) continue
      wipe(f)
    }
  }

  /** 与 Go paths.IsLegacyTVDir 同规则，勿另写一套。 */
  private fun isLegacyTVDir(name: String): Boolean {
    val low = name.lowercase()
    if (low.startsWith("s_c_")) return true
    if (!low.startsWith("s_u_")) return false
    val rest = low.removePrefix("s_u_")
    if (rest.length != 32) return false
    return rest.all { it in '0'..'9' || it in 'a'..'f' }
  }
}
