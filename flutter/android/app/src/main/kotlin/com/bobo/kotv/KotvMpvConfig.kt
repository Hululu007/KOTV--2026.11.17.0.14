package com.bobo.kotv

import android.content.Context
import android.util.Log
import androidx.media3.common.util.Util
import androidx.media3.mpvplayer.MpvAndroidOptions
import androidx.media3.mpvplayer.MpvPlayerConfig
import androidx.media3.mpvplayer.MpvSubtitleOptions
import java.io.File

/** Media3 MpvPlayer 配置与托管 conf 键（插件会话外收拢）。 */
internal object KotvMpvConfig {
  private const val TAG = "KotvMpvConfig"

  val MANAGED_CONF_OPTIONS: Set<String> =
    setOf(
      "vo",
      "wid",
      "android-surface-size",
      "gpu-api",
      "gpu-context",
      "hwdec",
      "audio-spdif",
      "android-dolby-vision-output",
      "demuxer-dovi-profile7",
      "cache",
      "cache-on-disk",
      "demuxer-cache-dir",
      "cache-secs",
      "demuxer-max-bytes",
      "demuxer-max-back-bytes",
      "cache-pause",
      "cache-pause-initial",
      "tls-verify",
      "tls-ca-file",
      "slang",
      "brightness",
      "contrast",
      "saturation",
      "gamma",
      "hue",
    )

  fun defaultUserAgent(ctx: Context): String = Util.getUserAgent(ctx, ctx.packageName)

  fun build(
    ctx: Context,
    tlsVerify: Boolean,
    diskCache: Boolean,
    livePlayback: Boolean,
    audioPassThrough: Boolean,
    gpuNext: Boolean,
    vulkanEnabled: Boolean,
    dolbyVisionPolicy: Int,
    conf: String,
    subtitleOptions: MpvSubtitleOptions,
    resolveDolbyVisionPolicy: (Int) -> Int,
    bufferBytes: Int = 0,
  ): MpvPlayerConfig {
    val configDir = File(ctx.filesDir, "mpv").apply { mkdirs() }
    val cacheDir = File(ctx.cacheDir, "mpv").apply { mkdirs() }
    KotvFontConfig.prepare(configDir, cacheDir)
    val androidOpts =
      MpvAndroidOptions.Builder()
        .setShaderCacheDirectory(cacheDir)
        .setAudioPassthroughEnabled(audioPassThrough)
        .setGpuNextEnabled(gpuNext)
        .setVulkanEnabled(vulkanEnabled)
        .setDolbyVisionOutputPolicy(resolveDolbyVisionPolicy(dolbyVisionPolicy))
        .build()
    val builder =
      MpvPlayerConfig.Builder()
        .setDefaultUserAgent(defaultUserAgent(ctx))
        .addAndroidDefaults(androidOpts)
        .addConfigDirectory(configDir)
        .addAndroidFontConfig(configDir, cacheDir)
        .addAndroidSubtitleOptions(ctx, subtitleOptions)
    if (tlsVerify) {
      val ca = File(ctx.filesDir, "cacert.pem")
      builder.addTlsCaFileFromAsset(ctx, "cacert.pem", ca)
    } else {
      builder.addPreInitStringOption("tls-verify", "no")
    }
    // 点播/直播共用 demuxer 字节预算；直播另关 cache-pause，避免 paused-for-cache 卡几秒。
    for ((k, v) in KotvMedia3Playback.mpvDemuxerByteProps(bufferBytes)) {
      builder.addPreInitStringOption(k, v)
    }
    if (livePlayback) {
      builder.addPreInitStringOption("cache-pause", "no")
      builder.addPreInitStringOption("cache-on-disk", "no")
    } else if (diskCache) {
      val dir = File(ctx.cacheDir, "mpv-cache").apply { mkdirs() }
      builder.addDiskCacheOptions(dir, 3600)
    }
    applyConfToBuilder(builder, conf)
    return builder.build()
  }

  fun applyConfToBuilder(builder: MpvPlayerConfig.Builder, text: String) {
    for (raw in text.split(Regex("[\r\n]+"))) {
      var line = raw.trim()
      if (line.isEmpty() || line.startsWith("#")) continue
      val hash = line.indexOf('#')
      if (hash > 0) line = line.substring(0, hash).trim()
      if (line.isEmpty()) continue
      val key: String
      val value: String
      val eq = line.indexOf('=')
      if (eq > 0) {
        key = line.substring(0, eq).trim()
        value = line.substring(eq + 1).trim()
      } else {
        val sp = Regex("\\s+").find(line) ?: continue
        key = line.substring(0, sp.range.first).trim()
        value = line.substring(sp.range.last + 1).trim()
      }
      if (key.isEmpty() || key.startsWith("kotv-")) continue
      val base = if (key.startsWith("no-")) key.substring(3) else key
      if (MANAGED_CONF_OPTIONS.contains(key) || MANAGED_CONF_OPTIONS.contains(base)) continue
      try {
        builder.addAppOwnedStringOption(key, value.ifEmpty { "yes" })
      } catch (t: Throwable) {
        Log.w(TAG, "skip conf option $key", t)
      }
    }
  }
}
