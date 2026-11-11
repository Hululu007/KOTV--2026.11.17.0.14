package com.bobo.kotv

import android.content.Context
import android.graphics.Color
import android.graphics.PixelFormat
import android.graphics.SurfaceTexture
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.Gravity
import android.view.Surface
import android.view.SurfaceHolder
import android.view.SurfaceView
import android.view.TextureView
import android.view.View
import android.view.ViewGroup
import android.view.accessibility.CaptioningManager
import android.widget.FrameLayout
import androidx.media3.common.C
import androidx.media3.common.Format
import androidx.media3.common.MediaChapter
import androidx.media3.common.MediaEdition
import androidx.media3.common.MediaItem
import androidx.media3.common.MimeTypes
import androidx.media3.common.PlaybackException
import androidx.media3.common.PlaybackParameters
import androidx.media3.common.Player
import androidx.media3.common.TrackSelectionOverride
import androidx.media3.common.Tracks
import androidx.media3.common.VideoSize
import androidx.media3.mpvplayer.MpvAndroidOptions
import androidx.media3.mpvplayer.MpvPlayer
import androidx.media3.mpvplayer.MpvPlayerConfig
import androidx.media3.mpvplayer.MpvSubtitleOptions
import androidx.media3.mpvplayer.audio.MpvAudioFilter
import androidx.media3.mpvplayer.video.MpvVideoEqualizer
import io.flutter.embedding.engine.plugins.FlutterPlugin
import io.flutter.plugin.common.EventChannel
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import io.flutter.plugin.common.StandardMessageCodec
import io.flutter.plugin.platform.PlatformView
import io.flutter.plugin.platform.PlatformViewFactory
import java.io.File
import kotlin.math.roundToInt
import org.json.JSONArray
import org.json.JSONObject

/**
 * Android 原生 MPV：Media3 [MpvPlayer] + PlatformView SurfaceView/TextureView。
 * MethodChannel `kotv_mpv` / EventChannel `kotv_mpv/events` / viewType `kotv_mpv/surface`。
 */
class KotvMpvPlugin : FlutterPlugin, MethodChannel.MethodCallHandler, EventChannel.StreamHandler {

  private var channel: MethodChannel? = null
  private var events: EventChannel? = null
  private var eventSink: EventChannel.EventSink? = null
  private var appContext: Context? = null
  private var surfaceHost: KotvMpvSurfaceHost? = null

  private val main = Handler(Looper.getMainLooper())
  private var player: MpvPlayer? = null
  private var playerListener: Player.Listener? = null

  private var playing = false
  private var buffering = false
  private var repeatOne = false
  private var livePlayback = false
  private var decodeMode = "auto"
  private var gpuNext = false
  private var vulkanEnabled = false
  private var conf = ""
  private var tlsVerify = true
  private var audioPassThrough = true
  private var diskCache = false
  private var dolbyVisionPolicy = 0
  private var preferredTextLangs = ""
  private var volumePct = 80.0
  private var rate = 1.0
  private var renderTexture = false
  private var surfaceLayerEnabled = false
  private var aspectMode = "default"
  private var videoWidth = 0
  private var videoHeight = 0
  private var videoAspect = 0.0

  private var currentUrl = ""
  private var currentHeaders: Map<String, String> = emptyMap()
  private var pendingStartAtMs = 0L

  private var eqBrightness = 0
  private var eqContrast = 0
  private var eqSaturation = 0
  private var eqGamma = 0
  private var eqHue = 0
  private var eqSharpness = 0.0
  private var audioAf = ""

  private var subtitleScale = 1.0
  private var subtitlePos = 0.0
  private var subtitleSecondaryPos = 10.0
  private var subtitleForceStyle = false
  private var subtitleUseSystemStyle = false
  private var subtitleColor = "#FFFFFF"
  private var subtitleBorderColor = "#000000"
  private var subtitleBorderSize = 2.0
  private var subtitleBgColor = "#00000000"
  private var subtitleEdgeType = "outline"
  private var subtitleTextOpacity = 100.0
  private var subtitleBgOpacity = 100.0
  private var subtitleEdgeOpacity = 100.0
  private var subtitleShadowStrength = 50.0
  private var subtitleOffsetMs = 0L
  private var secondarySubtitleId = "no"

  /** 配置指纹：变化时重建 [MpvPlayer]。 */
  private var builtConfigKey: String? = null
  private var boundSurfaceView: SurfaceView? = null
  private var boundTextureView: TextureView? = null
  private var boundOsdSurfaceView: SurfaceView? = null

  private val tick = object : Runnable {
    override fun run() {
      val p = player ?: return
      try {
        val pos = p.currentPosition.coerceAtLeast(0L)
        val dur = p.duration.coerceAtLeast(0L)
        val buf = p.bufferedPosition.coerceAtLeast(0L)
        val isPlaying = p.isPlaying
        val isBuf =
          p.playbackState == Player.STATE_BUFFERING ||
            (p.playbackState == Player.STATE_READY && !isPlaying && p.playWhenReady)
        playing = isPlaying
        buffering = isBuf
        emit(
          mapOf(
            "event" to "position",
            "positionMs" to pos,
            "durationMs" to dur,
            "bufferedMs" to buf,
            "playing" to isPlaying,
            "buffering" to isBuf,
            "speedBps" to 0L,
          ),
        )
      } catch (t: Throwable) {
        Log.w(TAG, "tick", t)
      }
      main.postDelayed(this, 300)
    }
  }

  override fun onAttachedToEngine(binding: FlutterPlugin.FlutterPluginBinding) {
    appContext = binding.applicationContext
    channel = MethodChannel(binding.binaryMessenger, "kotv_mpv").also {
      it.setMethodCallHandler(this)
    }
    events = EventChannel(binding.binaryMessenger, "kotv_mpv/events").also {
      it.setStreamHandler(this)
    }
    binding.platformViewRegistry.registerViewFactory(
      VIEW_TYPE,
      KotvMpvSurfaceFactory(this),
    )
    Log.i(TAG, "KotvMpvPlugin attached (Media3 MpvPlayer)")
  }

  override fun onDetachedFromEngine(binding: FlutterPlugin.FlutterPluginBinding) {
    releasePlayer()
    channel?.setMethodCallHandler(null)
    channel = null
    events?.setStreamHandler(null)
    events = null
    eventSink = null
    appContext = null
  }

  override fun onListen(arguments: Any?, events: EventChannel.EventSink?) {
    eventSink = events
  }

  override fun onCancel(arguments: Any?) {
    eventSink = null
  }

  internal fun attachSurfaceHost(host: KotvMpvSurfaceHost) {
    surfaceHost = host
    host.onSurface = { ready ->
      if (ready) {
        bindPlayerSurface()
        syncOsdSurface()
      } else {
        unbindPlayerOutput(host, transient = true)
      }
    }
    host.onSurfaceSize = { _, _ ->
      if (surfaceHost === host) applyVideoFrame()
    }
    host.onOsdSurface = {
      if (surfaceHost === host) syncOsdSurface()
    }
    host.setOsdLayer(!renderTexture)
    if (surfaceLayerEnabled) {
      host.setRender(renderTexture)
      if (host.surfaceView != null || host.textureView != null) {
        bindPlayerSurface()
        syncOsdSurface()
      }
    } else {
      host.clearVideoLayer()
    }
  }

  internal fun detachSurfaceHost(host: KotvMpvSurfaceHost) {
    if (surfaceHost === host) {
      unbindPlayerOutput(host, transient = false)
      host.release()
      surfaceHost = null
    }
  }

  override fun onMethodCall(call: MethodCall, result: MethodChannel.Result) {
    when (call.method) {
      "isVulkanAvailable" -> {
        val ctx = appContext
        result.success(
          ctx != null &&
            KotvMpvNativeBootstrap.isDeviceVulkanCapable(ctx) &&
            !KotvMpvNativeBootstrap.isVulkanStubLoaded(),
        )
      }
      "create" -> {
        ingestPlayerArgs(call)
        call.argument<Boolean>("live")?.let { livePlayback = it }
        call.argument<Boolean>("repeatOne")?.let { repeatOne = it }
        call.argument<String>("render")?.trim()?.takeIf { it.isNotEmpty() }?.let {
          renderTexture = resolveRenderTexture(it)
        }
        main.post {
          try {
            if (surfaceLayerEnabled) {
              surfaceHost?.setRender(renderTexture)
            } else {
              surfaceHost?.clearVideoLayer()
            }
            ensurePlayer()
            applyRepeatOne()
            result.success(mapOf("ok" to true, "ready" to (player != null)))
          } catch (e: Throwable) {
            emit(mapOf("event" to "error", "message" to (e.message ?: "create failed")))
            result.error("CREATE_FAILED", e.message, null)
          }
        }
      }
      "open" -> {
        val url = call.argument<String>("url")?.trim().orEmpty()
        if (url.isEmpty()) {
          result.error("OPEN_FAILED", "empty url", null)
          return
        }
        val headers = stringMapArg(call, "headers")
        livePlayback = call.argument<Boolean>("live") == true
        ingestPlayerArgs(call)
        ingestEqualizerArgs(call)
        call.argument<String>("render")?.trim()?.takeIf { it.isNotEmpty() }?.let {
          renderTexture = resolveRenderTexture(it)
        }
        pendingStartAtMs = call.argument<Number>("startAtMs")?.toLong()?.takeIf { it > 0L } ?: 0L
        main.post {
          try {
            surfaceHost?.let { host ->
              if (surfaceLayerEnabled) host.setRender(renderTexture) else host.clearVideoLayer()
            }
            openInternal(url, headers, pendingStartAtMs)
            result.success(null)
          } catch (e: Throwable) {
            emit(mapOf("event" to "error", "message" to (e.message ?: "open failed")))
            result.error("OPEN_FAILED", e.message, null)
          }
        }
      }
      "play" -> {
        main.post {
          try {
            player?.play()
            playing = true
            result.success(null)
          } catch (e: Throwable) {
            result.error("PLAY_FAILED", e.message, null)
          }
        }
      }
      "pause" -> {
        main.post {
          try {
            player?.pause()
            playing = false
            result.success(null)
          } catch (e: Throwable) {
            result.error("PAUSE_FAILED", e.message, null)
          }
        }
      }
      "stop" -> {
        main.post {
          try {
            player?.stop()
            player?.clearMediaItems()
            playing = false
            buffering = false
            currentUrl = ""
            videoWidth = 0
            videoHeight = 0
            result.success(null)
          } catch (e: Throwable) {
            result.error("STOP_FAILED", e.message, null)
          }
        }
      }
      "seek" -> {
        val ms = (call.argument<Number>("positionMs") ?: 0).toLong().coerceAtLeast(0L)
        main.post {
          try {
            player?.seekTo(ms)
            result.success(null)
          } catch (e: Throwable) {
            result.error("SEEK_FAILED", e.message, null)
          }
        }
      }
      "setVolume" -> {
        val raw = (call.argument<Number>("volume") ?: 80).toDouble()
        volumePct = raw.coerceIn(0.0, 100.0)
        main.post {
          try {
            player?.volume = (volumePct / 100.0).toFloat().coerceIn(0f, 1f)
            result.success(null)
          } catch (e: Throwable) {
            result.error("VOLUME_FAILED", e.message, null)
          }
        }
      }
      "setRate" -> {
        rate = (call.argument<Number>("rate") ?: 1).toDouble().coerceIn(0.25, 4.0)
        main.post {
          try {
            player?.playbackParameters = PlaybackParameters(rate.toFloat())
            result.success(null)
          } catch (e: Throwable) {
            result.error("RATE_FAILED", e.message, null)
          }
        }
      }
      "setRepeatOne" -> {
        repeatOne = call.argument<Boolean>("on") == true
        main.post {
          try {
            applyRepeatOne()
            result.success(null)
          } catch (e: Throwable) {
            result.error("REPEAT_FAILED", e.message, null)
          }
        }
      }
      "setDecode" -> {
        decodeMode = call.argument<String>("decode") ?: decodeMode
        main.post {
          try {
            player?.setDecode(resolveDecodeMode(decodeMode))
            result.success(null)
          } catch (e: Throwable) {
            result.error("DECODE_FAILED", e.message, null)
          }
        }
      }
      "setRenderMode" -> {
        val mode = call.argument<String>("mode") ?: ""
        main.post {
          try {
            renderTexture = resolveRenderTexture(mode)
            boundSurfaceView = null
            boundTextureView = null
            if (surfaceLayerEnabled) {
              surfaceHost?.setRender(renderTexture)
              surfaceHost?.setOsdLayer(!renderTexture)
              bindPlayerSurface()
              syncOsdSurface()
            }
            result.success(
              mapOf(
                "ok" to true,
                "render" to if (renderTexture) "texture" else "surface",
                "path" to "platformView",
              ),
            )
          } catch (e: Throwable) {
            result.error("RENDER_FAILED", e.message, null)
          }
        }
      }
      "setSurfaceLayerEnabled" -> {
        val enabled = call.argument<Boolean>("enabled") == true
        main.post {
          try {
            setSurfaceLayerEnabled(enabled)
            result.success(true)
          } catch (e: Throwable) {
            result.error("SURFACE_LAYER_FAILED", e.message, null)
          }
        }
      }
      "setOpts" -> {
        ingestPlayerArgs(call)
        ingestEqualizerArgs(call)
        main.post {
          try {
            val url = currentUrl
            val headers = currentHeaders
            val pos = player?.currentPosition?.coerceAtLeast(0L) ?: 0L
            ensurePlayer(forceRebuild = true)
            if (url.isNotBlank()) {
              openInternal(url, headers, pos)
            } else {
              applyEqualizer()
              applySubtitleStyle()
              applySubtitleOffset()
            }
            result.success(null)
          } catch (e: Throwable) {
            result.error("OPTS_FAILED", e.message, null)
          }
        }
      }
      "setEqualizer" -> {
        ingestEqualizerArgs(call)
        main.post {
          try {
            applyEqualizer()
            result.success(null)
          } catch (e: Throwable) {
            result.error("EQ_FAILED", e.message, null)
          }
        }
      }
      "setAspect" -> {
        val mode = call.argument<String>("mode")?.trim().orEmpty()
        main.post {
          try {
            aspectMode = normalizeAspectMode(mode)
            applyVideoFrame()
            result.success(true)
          } catch (e: Throwable) {
            result.error("ASPECT_FAILED", e.message, null)
          }
        }
      }
      "setAudioTrack" -> {
        val id = call.argument<String>("id") ?: ""
        main.post {
          try {
            selectTrackById(C.TRACK_TYPE_AUDIO, id)
            result.success(null)
          } catch (e: Throwable) {
            result.error("AUDIO_FAILED", e.message, null)
          }
        }
      }
      "getAudioTracks" -> {
        main.post {
          try {
            result.success(buildTracksJson(C.TRACK_TYPE_AUDIO))
          } catch (e: Throwable) {
            result.error("TRACKS_FAILED", e.message, null)
          }
        }
      }
      "getVideoTracks" -> {
        main.post {
          try {
            result.success(buildTracksJson(C.TRACK_TYPE_VIDEO))
          } catch (e: Throwable) {
            result.error("TRACKS_FAILED", e.message, null)
          }
        }
      }
      "getSubtitleTracks" -> {
        main.post {
          try {
            result.success(buildTracksJson(C.TRACK_TYPE_TEXT))
          } catch (e: Throwable) {
            result.error("TRACKS_FAILED", e.message, null)
          }
        }
      }
      "setVideoTrack" -> {
        val id = call.argument<String>("id") ?: ""
        main.post {
          try {
            selectTrackById(C.TRACK_TYPE_VIDEO, id)
            result.success(null)
          } catch (e: Throwable) {
            result.error("VIDEO_FAILED", e.message, null)
          }
        }
      }
      "setSubtitleTrack" -> {
        val id = call.argument<String>("id") ?: ""
        main.post {
          try {
            selectSubtitleTrackById(id)
            result.success(null)
          } catch (e: Throwable) {
            result.error("SUB_FAILED", e.message, null)
          }
        }
      }
      "setSecondarySubtitleTrack" -> {
        val id = call.argument<String>("id") ?: "no"
        secondarySubtitleId = id
        main.post {
          try {
            applySubtitleStyle()
            result.success(null)
          } catch (e: Throwable) {
            result.error("SECONDARY_SUB_FAILED", e.message, null)
          }
        }
      }
      "setSubtitleStyle" -> {
        call.argument<Number>("scale")?.toDouble()?.let {
          subtitleScale = it.coerceIn(0.5, 2.0)
        }
        call.argument<Number>("pos")?.toDouble()?.let { subtitlePos = it }
        call.argument<Number>("secondaryPos")?.toDouble()?.let {
          subtitleSecondaryPos = it.coerceIn(0.0, 150.0)
        }
        call.argument<Boolean>("forceStyle")?.let { subtitleForceStyle = it }
        call.argument<Boolean>("useSystemStyle")?.let { subtitleUseSystemStyle = it }
        call.argument<String>("color")?.trim()?.takeIf { it.isNotEmpty() }?.let { subtitleColor = it }
        call.argument<String>("borderColor")?.trim()?.takeIf { it.isNotEmpty() }?.let {
          subtitleBorderColor = it
        }
        call.argument<Number>("borderSize")?.toDouble()?.let {
          subtitleBorderSize = it.coerceIn(0.0, 8.0)
        }
        call.argument<String>("bgColor")?.trim()?.takeIf { it.isNotEmpty() }?.let {
          subtitleBgColor = it
        }
        call.argument<String>("edgeType")?.trim()?.takeIf { it.isNotEmpty() }?.let {
          subtitleEdgeType = it.lowercase()
        }
        call.argument<Number>("textOpacity")?.toDouble()?.let {
          subtitleTextOpacity = it.coerceIn(0.0, 100.0)
        }
        call.argument<Number>("bgOpacity")?.toDouble()?.let {
          subtitleBgOpacity = it.coerceIn(0.0, 100.0)
        }
        call.argument<Number>("edgeOpacity")?.toDouble()?.let {
          subtitleEdgeOpacity = it.coerceIn(0.0, 100.0)
        }
        call.argument<Number>("shadowStrength")?.toDouble()?.let {
          subtitleShadowStrength = it.coerceIn(0.0, 100.0)
        }
        main.post {
          try {
            applySubtitleStyle()
            result.success(null)
          } catch (e: Throwable) {
            result.error("SUB_STYLE_FAILED", e.message, null)
          }
        }
      }
      "setSubtitleOffsetMs" -> {
        subtitleOffsetMs = (call.argument<Number>("ms")?.toLong() ?: 0L).coerceIn(-300_000L, 300_000L)
        main.post {
          try {
            applySubtitleOffset()
            result.success(null)
          } catch (e: Throwable) {
            result.error("SUB_OFFSET_FAILED", e.message, null)
          }
        }
      }
      "discTitleCount" -> {
        main.post {
          try {
            result.success(player?.currentMediaEditions?.size ?: 0)
          } catch (e: Throwable) {
            result.error("DISC_FAILED", e.message, null)
          }
        }
      }
      "discChapterCount" -> {
        main.post {
          try {
            result.success(player?.currentMediaChapters?.size ?: 0)
          } catch (e: Throwable) {
            result.error("DISC_FAILED", e.message, null)
          }
        }
      }
      "setDiscTitle" -> {
        val index = call.argument<Number>("index")?.toInt() ?: -1
        main.post {
          try {
            val editions = player?.currentMediaEditions.orEmpty()
            val edition = editions.firstOrNull { it.index == index }
            if (edition != null) {
              player?.selectEdition(edition)
            }
            result.success(null)
          } catch (e: Throwable) {
            result.error("DISC_FAILED", e.message, null)
          }
        }
      }
      "setDiscChapter" -> {
        val index = call.argument<Number>("index")?.toInt() ?: -1
        main.post {
          try {
            val chapters = player?.currentMediaChapters.orEmpty()
            val chapter = chapters.firstOrNull { it.index == index }
            if (chapter != null) {
              player?.selectChapter(chapter)
            }
            result.success(null)
          } catch (e: Throwable) {
            result.error("DISC_FAILED", e.message, null)
          }
        }
      }
      "addSubtitle" -> {
        val path = call.argument<String>("path")?.trim().orEmpty()
        val title = call.argument<String>("title")?.trim().orEmpty()
        main.post {
          try {
            val p = player
            if (p == null || path.isEmpty()) {
              result.error("SUB_FAILED", "empty path or not ready", null)
              return@post
            }
            val uri = if (path.startsWith("/")) Uri.fromFile(File(path)) else Uri.parse(path)
            val builder = MediaItem.SubtitleConfiguration.Builder(uri)
            if (title.isNotEmpty()) builder.setLabel(title)
            mimeFromPath(path)?.let { builder.setMimeType(it) }
            val ok = p.addSubtitle(builder.build())
            result.success(ok)
          } catch (e: Throwable) {
            result.error("SUB_FAILED", e.message, null)
          }
        }
      }
      "videoInfo" -> {
        main.post {
          try {
            result.success(videoInfo())
          } catch (e: Throwable) {
            result.error("VIDEO_INFO_FAILED", e.message, null)
          }
        }
      }
      "retryVideo" -> {
        main.post {
          try {
            val p = player
            val url = currentUrl
            if (p != null && url.isNotBlank()) {
              val pos = p.currentPosition.coerceAtLeast(0L)
              openInternal(url, currentHeaders, pos)
            } else {
              p?.prepare()
              p?.play()
            }
            result.success(null)
          } catch (e: Throwable) {
            result.error("RETRY_FAILED", e.message, null)
          }
        }
      }
      "dispose" -> {
        main.post {
          releasePlayer()
          result.success(null)
        }
      }
      else -> result.notImplemented()
    }
  }

  private fun openInternal(url: String, headers: Map<String, String>, startPositionMs: Long) {
    currentUrl = url
    currentHeaders = headers
    ensurePlayer()
    val p = player ?: error("mpv player missing")
    applyTrackSelectionPrefs()
    applyEqualizer()
    applySubtitleStyle()
    applySubtitleOffset()
    applyRepeatOne()
    p.volume = (volumePct / 100.0).toFloat().coerceIn(0f, 1f)
    p.playbackParameters = PlaybackParameters(rate.toFloat())
    bindPlayerSurface()
    syncOsdSurface()
    val mediaItem = buildMediaItem(url, headers)
    if (startPositionMs > 0L) {
      p.setMediaItem(mediaItem, startPositionMs)
    } else {
      p.setMediaItem(mediaItem, true)
    }
    p.prepare()
    p.play()
    main.removeCallbacks(tick)
    main.post(tick)
  }

  private fun ensurePlayer(forceRebuild: Boolean = false) {
    val ctx = appContext ?: error("no context")
    if (!MpvPlayer.isAvailable()) {
      throw IllegalStateException("MpvPlayer native libraries unavailable")
    }
    val key = configKey()
    if (!forceRebuild && player != null && builtConfigKey == key) {
      bindPlayerSurface()
      return
    }
    releasePlayerInstance()
    val config = buildConfig(ctx)
    val p =
      MpvPlayer.Builder(ctx)
        .setConfig(config)
        .setDecode(resolveDecodeMode(decodeMode))
        .setLooper(Looper.getMainLooper())
        .build()
    attachListener(p)
    player = p
    builtConfigKey = key
    applyTrackSelectionPrefs()
    applyEqualizer()
    applySubtitleStyle()
    applySubtitleOffset()
    applyRepeatOne()
    p.volume = (volumePct / 100.0).toFloat().coerceIn(0f, 1f)
    p.playbackParameters = PlaybackParameters(rate.toFloat())
    bindPlayerSurface()
    syncOsdSurface()
    Log.i(TAG, "MpvPlayer ready key=$key forceRebuild=$forceRebuild")
    main.removeCallbacks(tick)
    main.post(tick)
  }

  private fun configKey(): String =
    listOf(
      livePlayback,
      tlsVerify,
      diskCache && !livePlayback,
      audioPassThrough,
      gpuNext,
      vulkanEnabled,
      dolbyVisionPolicy,
      conf.hashCode(),
    ).joinToString("|")

  private fun buildConfig(ctx: Context): MpvPlayerConfig {
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
        .setDefaultUserAgent(defaultUserAgent())
        .addAndroidDefaults(androidOpts)
        .addConfigDirectory(configDir)
        .addAndroidFontConfig(configDir, cacheDir)
        .addAndroidSubtitleOptions(ctx, buildSubtitleOptions())
    if (tlsVerify) {
      val ca = File(ctx.filesDir, "cacert.pem")
      builder.addTlsCaFileFromAsset(ctx, "cacert.pem", ca)
    } else {
      builder.addPreInitStringOption("tls-verify", "no")
    }
    if (!livePlayback && diskCache) {
      val dir = File(ctx.cacheDir, "mpv-cache").apply { mkdirs() }
      builder.addDiskCacheOptions(dir, 3600)
    }
    applyConfToBuilder(builder, conf)
    return builder.build()
  }

  private fun applyConfToBuilder(builder: MpvPlayerConfig.Builder, text: String) {
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

  private fun buildMediaItem(url: String, headers: Map<String, String>): MediaItem {
    val uri = playUri(url)
    val b = MediaItem.Builder().setUri(uri).setMediaId(url)
    if (headers.isNotEmpty()) {
      val extras = Bundle()
      headers.forEach { (k, v) -> extras.putString(k, v) }
      b.setRequestMetadata(
        MediaItem.RequestMetadata.Builder()
          .setMediaUri(uri)
          .setExtras(extras)
          .build(),
      )
      b.setCustomCacheKey(cacheKeyFor(url, headers))
    }
    return b.build()
  }

  private fun playUri(url: String): Uri {
    val trimmed = url.trim()
    return when {
      trimmed.startsWith("content:", ignoreCase = true) ||
        trimmed.startsWith("file:", ignoreCase = true) ||
        trimmed.startsWith("http:", ignoreCase = true) ||
        trimmed.startsWith("https:", ignoreCase = true) ||
        trimmed.startsWith("rtsp:", ignoreCase = true) ||
        trimmed.startsWith("rtmp:", ignoreCase = true) -> Uri.parse(trimmed)
      trimmed.startsWith("/") -> Uri.fromFile(File(trimmed))
      else -> Uri.parse(trimmed)
    }
  }

  private fun cacheKeyFor(url: String, headers: Map<String, String>): String {
    val ua = headers.entries.firstOrNull { it.key.equals("User-Agent", true) }?.value.orEmpty()
    val ref = headers.entries.firstOrNull { it.key.equals("Referer", true) }?.value.orEmpty()
    return "$url|$ua|$ref"
  }

  private fun attachListener(p: MpvPlayer) {
    val listener =
      object : Player.Listener {
        override fun onTracksChanged(tracks: Tracks) {
          emit(mapOf("event" to "tracks"))
        }

        override fun onPlaybackStateChanged(playbackState: Int) {
          when (playbackState) {
            Player.STATE_ENDED -> emit(mapOf("event" to "completed"))
            Player.STATE_READY -> {
              val vs = p.videoSize
              if (vs.width > 0 && vs.height > 0) {
                videoWidth = vs.width
                videoHeight = vs.height
                videoAspect = if (vs.height > 0) vs.width.toDouble() / vs.height else 0.0
                applyVideoFrame()
              }
              emit(
                mapOf(
                  "event" to "ready",
                  "width" to videoWidth,
                  "height" to videoHeight,
                  "durationMs" to p.duration.coerceAtLeast(0),
                ),
              )
            }
            Player.STATE_BUFFERING -> buffering = true
            Player.STATE_IDLE -> {}
          }
        }

        override fun onPlayerError(error: PlaybackException) {
          emit(mapOf("event" to "error", "message" to (error.message ?: error.errorCodeName)))
        }

        override fun onVideoSizeChanged(videoSize: VideoSize) {
          if (videoSize.width <= 0 || videoSize.height <= 0) return
          videoWidth = videoSize.width
          videoHeight = videoSize.height
          videoAspect = videoSize.width.toDouble() / videoSize.height
          applyVideoFrame()
          emit(
            mapOf(
              "event" to "size",
              "width" to videoWidth,
              "height" to videoHeight,
            ),
          )
        }

        override fun onMediaEditionsChanged(editions: List<MediaEdition>) {
          emit(mapOf("event" to "editions", "count" to editions.size))
        }

        override fun onMediaChaptersChanged(chapters: List<MediaChapter>) {
          emit(mapOf("event" to "chapters", "count" to chapters.size))
        }
      }
    playerListener = listener
    p.addListener(listener)
  }

  private fun setSurfaceLayerEnabled(enabled: Boolean) {
    if (surfaceLayerEnabled == enabled) {
      if (enabled) bindPlayerSurface()
      return
    }
    surfaceLayerEnabled = enabled
    val host = surfaceHost
    if (!enabled) {
      host?.let { unbindPlayerOutput(it, transient = false) }
      try {
        player?.clearVideoSurface()
      } catch (_: Throwable) {
      }
      clearOsd()
      host?.clearVideoLayer()
      return
    }
    if (host == null) return
    host.setRender(renderTexture)
    host.setOsdLayer(!renderTexture)
    bindPlayerSurface()
    syncOsdSurface()
  }

  private fun bindPlayerSurface() {
    val p = player ?: return
    if (!surfaceLayerEnabled) return
    val host = surfaceHost ?: return
    if (renderTexture) {
      val tv = host.textureView ?: return
      if (boundTextureView === tv) {
        applyVideoFrame()
        return
      }
      clearVideoOutputs(p)
      p.setVideoTextureView(tv)
      boundTextureView = tv
      boundSurfaceView = null
      clearOsd()
      applyVideoFrame()
      return
    }
    val sv = host.surfaceView ?: return
    if (!canBindSurface(sv)) return
    if (boundSurfaceView === sv) {
      applyVideoFrame()
      syncOsdSurface()
      return
    }
    clearVideoOutputs(p)
    p.setVideoSurfaceView(sv)
    boundSurfaceView = sv
    boundTextureView = null
    applyVideoFrame()
    syncOsdSurface()
  }

  private fun clearVideoOutputs(p: MpvPlayer) {
    try {
      boundSurfaceView?.let { p.clearVideoSurfaceView(it) }
    } catch (_: Throwable) {
    }
    try {
      boundTextureView?.let { p.clearVideoTextureView(it) }
    } catch (_: Throwable) {
    }
    try {
      p.clearVideoSurface()
    } catch (_: Throwable) {
    }
  }

  private fun unbindPlayerOutput(host: KotvMpvSurfaceHost, transient: Boolean) {
    val p = player
    if (p != null) {
      try {
        host.surfaceView?.let { p.clearVideoSurfaceView(it) }
      } catch (_: Throwable) {
      }
      try {
        host.textureView?.let { p.clearVideoTextureView(it) }
      } catch (_: Throwable) {
      }
      if (!transient) {
        try {
          p.clearVideoSurface()
        } catch (_: Throwable) {
        }
      }
    }
    clearOsd()
    if (boundSurfaceView === host.surfaceView) boundSurfaceView = null
    if (boundTextureView === host.textureView) boundTextureView = null
  }

  private fun syncOsdSurface() {
    val p = player ?: return
    if (renderTexture || !surfaceLayerEnabled) {
      clearOsd()
      return
    }
    val host = surfaceHost ?: return
    val osd = host.osdSurfaceView
    if (osd == null || !host.osdReady) {
      clearOsd()
      return
    }
    if (boundOsdSurfaceView === osd) return
    try {
      boundOsdSurfaceView?.let { p.clearOsdSurfaceView(it) }
    } catch (_: Throwable) {
    }
    p.setOsdSurfaceView(osd)
    boundOsdSurfaceView = osd
  }

  private fun clearOsd() {
    val p = player
    val prev = boundOsdSurfaceView
    boundOsdSurfaceView = null
    if (p != null && prev != null) {
      try {
        p.clearOsdSurfaceView(prev)
      } catch (_: Throwable) {
      }
    }
  }

  private fun canBindSurface(sv: SurfaceView): Boolean {
    val surface = sv.holder.surface ?: return false
    return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
      surface.isValid
    } else {
      true
    }
  }

  private fun applyVideoFrame() {
    val aspect =
      when (aspectMode) {
        "16:9" -> 16.0 / 9.0
        "4:3" -> 4.0 / 3.0
        else ->
          if (videoWidth > 0 && videoHeight > 0) {
            videoWidth.toDouble() / videoHeight
          } else {
            videoAspect
          }
      }
    surfaceHost?.setVideoFrame(aspect, aspectMode)
  }

  private fun applyRepeatOne() {
    player?.repeatMode =
      if (repeatOne) Player.REPEAT_MODE_ONE else Player.REPEAT_MODE_OFF
  }

  private fun applyTrackSelectionPrefs() {
    val p = player ?: return
    try {
      val builder = p.trackSelectionParameters.buildUpon()
      val langs = preferredTextLanguageArray()
      if (langs.isNotEmpty()) {
        builder.setPreferredTextLanguages(*langs)
      }
      p.trackSelectionParameters = builder.build()
    } catch (t: Throwable) {
      Log.w(TAG, "applyTrackSelectionPrefs", t)
    }
  }

  private fun preferredTextLanguageArray(): Array<String> {
    val raw =
      preferredTextLangs.trim().ifEmpty {
        KotvLangUtil.preferredTextLanguages().joinToString(",")
      }
    if (raw.isEmpty()) return emptyArray()
    return raw
      .replace(';', ',')
      .split(',')
      .map { it.trim() }
      .filter { it.isNotEmpty() }
      .toTypedArray()
  }

  private fun applyEqualizer() {
    val p = player ?: return
    try {
      p.setVideoEqualizer(
        MpvVideoEqualizer.create(
          eqBrightness.toDouble(),
          eqContrast.toDouble(),
          eqSaturation.toDouble(),
          eqGamma.toDouble(),
          eqHue.toDouble(),
          (eqSharpness / 100.0).coerceIn(0.0, 1.0),
        ),
      )
    } catch (t: Throwable) {
      Log.w(TAG, "setVideoEqualizer", t)
    }
    try {
      val filter =
        if (audioAf.isBlank()) {
          MpvAudioFilter.EMPTY
        } else {
          MpvAudioFilter.create(audioAf, audioAf, emptyList())
        }
      p.setAudioFilter(filter)
    } catch (t: Throwable) {
      Log.w(TAG, "setAudioFilter", t)
    }
  }

  private fun applySubtitleOffset() {
    val p = player ?: return
    try {
      if (p.isCommandAvailable(Player.COMMAND_SET_TEXT_OFFSET)) {
        p.setTextOffsetMs(subtitleOffsetMs)
      }
    } catch (t: Throwable) {
      Log.w(TAG, "setTextOffsetMs", t)
    }
  }

  private fun applySubtitleStyle() {
    val p = player ?: return
    try {
      p.setSubtitleOptions(buildSubtitleOptions())
    } catch (t: Throwable) {
      Log.w(TAG, "setSubtitleOptions", t)
    }
  }

  private fun buildSubtitleOptions(): MpvSubtitleOptions {
    val b = MpvSubtitleOptions.Builder()
    b.setScale(subtitleScale.coerceIn(0.5, 2.0))
    // Flutter pos → mpv sub-pos（底=100）。
    b.setPosition((100.0 - subtitlePos).coerceIn(0.0, 150.0))
    val secondaryTrack = resolveSecondaryTrackId(secondarySubtitleId)
    b.setSecondarySubtitle(
      secondaryTrack,
      subtitleSecondaryPos.coerceIn(0.0, 150.0),
      subtitleForceStyle,
    )
    if (subtitleUseSystemStyle) {
      b.setSystemCaptionStyle()
    } else if (subtitleForceStyle) {
      val fg = parseColorWithOpacity(subtitleColor, subtitleTextOpacity)
      val bg = parseColorWithOpacity(subtitleBgColor, subtitleBgOpacity)
      val edge = parseColorWithOpacity(subtitleBorderColor, subtitleEdgeOpacity)
      val edgeType = mapEdgeType(subtitleEdgeType)
      val strength = (subtitleShadowStrength / 50.0).coerceIn(0.0, 2.5)
      val shadow =
        when (edgeType) {
          CaptioningManager.CaptionStyle.EDGE_TYPE_DROP_SHADOW -> 2.0 * strength
          CaptioningManager.CaptionStyle.EDGE_TYPE_RAISED,
          CaptioningManager.CaptionStyle.EDGE_TYPE_DEPRESSED -> 1.5 * strength
          else -> 0.0
        }
      val edgeSize =
        if (edgeType == CaptioningManager.CaptionStyle.EDGE_TYPE_OUTLINE) {
          subtitleBorderSize.coerceIn(0.0, 8.0)
        } else {
          0.0
        }
      b.setCustomStyle(fg, bg, edgeType, edge, edgeSize, shadow)
    }
    return b.build()
  }

  private fun resolveSecondaryTrackId(raw: String): Int {
    val key = raw.trim().lowercase()
    if (key.isEmpty() || key == "no" || key == "off" || key == "none") {
      return MpvSubtitleOptions.SECONDARY_TRACK_OFF
    }
    if (key == "auto") return MpvSubtitleOptions.SECONDARY_TRACK_AUTO
    val byFormat = findFormatId(C.TRACK_TYPE_TEXT, raw.trim())
    if (byFormat != null) return byFormat
    return raw.trim().toIntOrNull() ?: MpvSubtitleOptions.SECONDARY_TRACK_OFF
  }

  private fun findFormatId(type: Int, id: String): Int? {
    val p = player ?: return null
    val gMatch = Regex("""^g(\d+):t(\d+)$""").matchEntire(id)
    if (gMatch != null) {
      val gi = gMatch.groupValues[1].toInt()
      val ti = gMatch.groupValues[2].toInt()
      val groups = p.currentTracks.groups
      if (gi in groups.indices) {
        val g = groups[gi]
        if (g.type == type && ti in 0 until g.length) {
          return g.getTrackFormat(ti).id?.toIntOrNull()
        }
      }
    }
    for (g in p.currentTracks.groups) {
      if (g.type != type) continue
      for (i in 0 until g.length) {
        val f = g.getTrackFormat(i)
        if (f.id == id) return f.id?.toIntOrNull()
      }
    }
    return null
  }

  private fun selectTrackById(type: Int, id: String) {
    val p = player ?: return
    if (id.isEmpty() || id == "auto") {
      p.trackSelectionParameters =
        p.trackSelectionParameters
          .buildUpon()
          .clearOverridesOfType(type)
          .setTrackTypeDisabled(type, false)
          .build()
      p.play()
      return
    }
    val groups = p.currentTracks.groups
    var targetGroup: Tracks.Group? = null
    var trackIndex = -1
    val gMatch = Regex("""^g(\d+):t(\d+)$""").matchEntire(id)
    if (gMatch != null) {
      val gi = gMatch.groupValues[1].toInt()
      val ti = gMatch.groupValues[2].toInt()
      if (gi in groups.indices) {
        val g = groups[gi]
        if (g.type == type && ti in 0 until g.length && g.isTrackSupported(ti)) {
          targetGroup = g
          trackIndex = ti
        }
      }
    } else {
      for (g in groups) {
        if (g.type != type) continue
        for (i in 0 until g.length) {
          if (!g.isTrackSupported(i)) continue
          if (g.getTrackFormat(i).id == id) {
            targetGroup = g
            trackIndex = i
            break
          }
        }
        if (targetGroup != null) break
      }
    }
    val g = targetGroup ?: return
    if (trackIndex < 0) return
    p.trackSelectionParameters =
      p.trackSelectionParameters
        .buildUpon()
        .clearOverridesOfType(type)
        .setTrackTypeDisabled(type, false)
        .setOverrideForType(TrackSelectionOverride(g.mediaTrackGroup, trackIndex))
        .build()
    p.play()
  }

  private fun selectSubtitleTrackById(id: String) {
    val p = player ?: return
    val key = id.trim().lowercase()
    if (key.isEmpty() || key == "no" || key == "off") {
      p.trackSelectionParameters =
        p.trackSelectionParameters
          .buildUpon()
          .clearOverridesOfType(C.TRACK_TYPE_TEXT)
          .setTrackTypeDisabled(C.TRACK_TYPE_TEXT, true)
          .build()
      p.play()
      return
    }
    selectTrackById(C.TRACK_TYPE_TEXT, id.trim())
  }

  private fun buildTracksJson(type: Int): String {
    val p = player ?: return "[]"
    val arr = JSONArray()
    val groups = p.currentTracks.groups
    for (gi in groups.indices) {
      val g = groups[gi]
      if (g.type != type) continue
      for (i in 0 until g.length) {
        if (!g.isTrackSupported(i)) continue
        val f = g.getTrackFormat(i)
        val obj = JSONObject()
        val id = f.id?.takeIf { it.isNotBlank() } ?: "g$gi:t$i"
        obj.put("id", id)
        obj.put("title", f.label ?: "")
        obj.put("lang", f.language ?: "")
        obj.put("codec", f.codecs ?: "")
        if (f.width > 0 && f.height > 0) {
          obj.put("width", f.width)
          obj.put("height", f.height)
        }
        obj.put("selected", g.isTrackSelected(i))
        arr.put(obj)
      }
    }
    return arr.toString()
  }

  private fun videoInfo(): Map<String, String> {
    val display = appContext?.let { KotvDisplayHdr.label(it) } ?: ""
    val p = player
    if (p == null) return mapOf("display" to display)
    val f = selectedVideoFormat()
    val info = f?.colorInfo
    val dv = f != null && isDolbyVision(f)
    var transfer =
      when (info?.colorTransfer) {
        C.COLOR_TRANSFER_ST2084 -> "PQ"
        C.COLOR_TRANSFER_HLG -> "HLG"
        null, Format.NO_VALUE -> ""
        else -> "SDR"
      }
    var primaries =
      when (info?.colorSpace) {
        C.COLOR_SPACE_BT2020 -> "BT.2020"
        C.COLOR_SPACE_BT709 -> "BT.709"
        C.COLOR_SPACE_BT601 -> "BT.601"
        else -> ""
      }
    if (dv) {
      if (transfer.isEmpty() || transfer == "SDR") transfer = "PQ"
      if (primaries.isEmpty() || primaries == "BT.709") primaries = "BT.2020"
    }
    if (transfer.isEmpty()) transfer = "SDR"
    val hw =
      when (resolveDecodeMode(decodeMode)) {
        C.DECODE_HARDWARE -> "mediacodec"
        C.DECODE_SOFTWARE -> "no"
        else -> "auto"
      }
    return mapOf(
      "hwdec" to hw,
      "transfer" to transfer,
      "primaries" to primaries,
      "dolbyVision" to if (dv) "杜比视界" else "",
      "output" to "",
      "display" to display,
    )
  }

  private fun selectedVideoFormat(): Format? {
    val p = player ?: return null
    for (group in p.currentTracks.groups) {
      if (group.type != C.TRACK_TYPE_VIDEO) continue
      for (i in 0 until group.length) {
        if (!group.isTrackSelected(i)) continue
        return group.getTrackFormat(i)
      }
    }
    return null
  }

  private fun isDolbyVision(format: Format): Boolean {
    if (MimeTypes.VIDEO_DOLBY_VISION == format.sampleMimeType) return true
    val codecs = format.codecs?.lowercase().orEmpty()
    return codecs.startsWith("dvh1") || codecs.startsWith("dvhe") || codecs.startsWith("dav1")
  }

  private fun releasePlayer() {
    main.removeCallbacks(tick)
    releasePlayerInstance()
    currentUrl = ""
    currentHeaders = emptyMap()
    videoWidth = 0
    videoHeight = 0
  }

  private fun releasePlayerInstance() {
    val p = player
    player = null
    builtConfigKey = null
    boundSurfaceView = null
    boundTextureView = null
    boundOsdSurfaceView = null
    if (p != null) {
      try {
        playerListener?.let { p.removeListener(it) }
      } catch (_: Throwable) {
      }
      playerListener = null
      try {
        p.release()
      } catch (t: Throwable) {
        Log.w(TAG, "release", t)
      }
    }
  }

  private fun ingestPlayerArgs(call: MethodCall) {
    call.argument<String>("decode")?.let { decodeMode = it }
    call.argument<Boolean>("gpuNext")?.let { gpuNext = it }
    call.argument<Boolean>("vulkan")?.let { vulkanEnabled = it }
    call.argument<String>("conf")?.let { conf = it }
    call.argument<Boolean>("tlsVerify")?.let { tlsVerify = it }
    call.argument<Boolean>("audioPassThrough")?.let { audioPassThrough = it }
    call.argument<Boolean>("diskCache")?.let { diskCache = it }
    call.argument<Number>("dolbyVisionPolicy")?.toInt()?.let { dolbyVisionPolicy = it }
    call.argument<String>("preferredTextLangs")?.let { preferredTextLangs = it.trim() }
  }

  private fun ingestEqualizerArgs(call: MethodCall) {
    call.argument<Number>("eqBrightness")?.toInt()?.let { eqBrightness = it.coerceIn(-100, 100) }
    call.argument<Number>("eqContrast")?.toInt()?.let { eqContrast = it.coerceIn(-100, 100) }
    call.argument<Number>("eqSaturation")?.toInt()?.let { eqSaturation = it.coerceIn(-100, 100) }
    call.argument<Number>("eqGamma")?.toInt()?.let { eqGamma = it.coerceIn(-100, 100) }
    call.argument<Number>("eqHue")?.toInt()?.let { eqHue = it.coerceIn(-100, 100) }
    call.argument<Number>("eqSharpness")?.toDouble()?.let {
      eqSharpness = it.coerceIn(0.0, 100.0)
    }
    call.argument<String>("audioAf")?.let { audioAf = it.trim() }
    call.argument<String>("videoVf")?.let { vf ->
      if (call.argument<Number>("eqSharpness") == null) {
        eqSharpness = parseSharpnessFromVf(vf)
      }
    }
    call.argument<Number>("subtitleFontScale")?.toDouble()?.let {
      subtitleScale = it.coerceIn(0.5, 2.0)
    }
  }

  private fun parseSharpnessFromVf(vf: String): Double {
    // lavfi=[unsharp=5:5:AMOUNT:...]
    val m = Regex("""unsharp=5:5:([0-9.]+)""").find(vf) ?: return eqSharpness
    val amount = m.groupValues[1].toDoubleOrNull() ?: return eqSharpness
    return ((amount / 1.5) * 100.0).coerceIn(0.0, 100.0)
  }

  private fun stringMapArg(call: MethodCall, key: String): Map<String, String> =
    (call.argument<Map<*, *>>(key) ?: emptyMap<Any, Any>()).entries.associate {
      "${it.key}" to "${it.value}"
    }

  private fun resolveDecodeMode(raw: String?): @C.DecodeMode Int {
    val mode = raw?.trim()?.lowercase().orEmpty()
    return when (mode) {
      "no", "soft", "software", "sw" -> C.DECODE_SOFTWARE
      "mediacodec", "hard", "hardware", "hw" -> C.DECODE_HARDWARE
      else -> C.DECODE_HARDWARE // auto → 硬解优先（失败由内核回退）
    }
  }

  private fun resolveDolbyVisionPolicy(raw: Int): Int =
    when (raw) {
      1 -> androidx.media3.common.DolbyVisionOutputPolicy.ASSUME_SUPPORTED
      2 -> androidx.media3.common.DolbyVisionOutputPolicy.ASSUME_UNSUPPORTED
      else -> androidx.media3.common.DolbyVisionOutputPolicy.AUTO
    }

  private fun resolveRenderTexture(mode: String): Boolean {
    val m = mode.trim().lowercase()
    return m == "texture" || m == "textureview"
  }

  private fun normalizeAspectMode(raw: String): String =
    when (raw.trim().lowercase()) {
      "fill", "stretch" -> "fill"
      "zoom", "cover", "crop" -> "zoom"
      "16:9", "16/9" -> "16:9"
      "4:3", "4/3" -> "4:3"
      else -> "default"
    }

  private fun mapEdgeType(raw: String): Int =
    when (raw.trim().lowercase()) {
      "none" -> CaptioningManager.CaptionStyle.EDGE_TYPE_NONE
      "shadow" -> CaptioningManager.CaptionStyle.EDGE_TYPE_DROP_SHADOW
      "raised" -> CaptioningManager.CaptionStyle.EDGE_TYPE_RAISED
      "depressed" -> CaptioningManager.CaptionStyle.EDGE_TYPE_DEPRESSED
      else -> CaptioningManager.CaptionStyle.EDGE_TYPE_OUTLINE
    }

  private fun parseColorWithOpacity(raw: String, opacityPct: Double): Int {
    val base = parseColor(raw)
    val a = ((opacityPct.coerceIn(0.0, 100.0) / 100.0) * 255.0).roundToInt().coerceIn(0, 255)
    return (base and 0x00FFFFFF) or (a shl 24)
  }

  private fun parseColor(raw: String): Int {
    val s = raw.trim()
    return try {
      when {
        s.startsWith("#") -> Color.parseColor(s)
        s.startsWith("0x", true) -> Color.parseColor("#${s.substring(2)}")
        else -> Color.parseColor("#$s")
      }
    } catch (_: Throwable) {
      Color.WHITE
    }
  }

  private fun mimeFromPath(path: String): String? {
    val lower = path.lowercase()
    return when {
      lower.endsWith(".ass") || lower.endsWith(".ssa") -> MimeTypes.TEXT_SSA
      lower.endsWith(".vtt") -> MimeTypes.TEXT_VTT
      lower.endsWith(".srt") -> MimeTypes.APPLICATION_SUBRIP
      lower.endsWith(".ttml") || lower.endsWith(".xml") -> MimeTypes.APPLICATION_TTML
      else -> null
    }
  }

  private fun defaultUserAgent(): String =
    "KOTV/${Build.VERSION.RELEASE} (Linux; Android ${Build.VERSION.RELEASE})"

  private fun emit(payload: Map<String, Any?>) {
    val sink = eventSink ?: return
    main.post {
      try {
        sink.success(payload)
      } catch (t: Throwable) {
        Log.w(TAG, "emit", t)
      }
    }
  }

  companion object {
    private val MANAGED_CONF_OPTIONS =
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
    private const val TAG = "KotvMpv"
    const val VIEW_TYPE = "kotv_mpv/surface"
  }
}

internal class KotvMpvSurfaceFactory(
  private val plugin: KotvMpvPlugin,
) : PlatformViewFactory(StandardMessageCodec.INSTANCE) {
  override fun create(context: Context, viewId: Int, args: Any?): PlatformView {
    val host = KotvMpvSurfaceHost(context)
    plugin.attachSurfaceHost(host)
    return object : PlatformView {
      override fun getView(): View = host
      override fun dispose() {
        Handler(Looper.getMainLooper()).post {
          plugin.detachSurfaceHost(host)
        }
      }
    }
  }
}

/** SurfaceView / TextureView 宿主。 */
internal class KotvMpvSurfaceHost(context: Context) :
  FrameLayout(context),
  SurfaceHolder.Callback,
  TextureView.SurfaceTextureListener {

  var surfaceView: SurfaceView? = null
    private set
  var textureView: TextureView? = null
    private set
  private var textureSurface: Surface? = null
  private var useTexture = false
  var onSurface: ((Boolean) -> Unit)? = null
  var onSurfaceSize: ((Int, Int) -> Unit)? = null
  var bufferSize: String? = null
    private set
  var onOsdSurface: (() -> Unit)? = null
  var osdSize: String? = null
    private set
  var osdReady = false
    private set
  var osdSurfaceView: SurfaceView? = null
    private set
  private var osdWanted = false
  private val osdCallback =
    object : SurfaceHolder.Callback {
      override fun surfaceCreated(holder: SurfaceHolder) {
        osdReady = true
        onOsdSurface?.invoke()
      }

      override fun surfaceChanged(holder: SurfaceHolder, format: Int, width: Int, height: Int) {
        if (width <= 0 || height <= 0) return
        osdSize = "${width}x$height"
        onOsdSurface?.invoke()
      }

      override fun surfaceDestroyed(holder: SurfaceHolder) {
        osdReady = false
        osdSize = null
        onOsdSurface?.invoke()
      }
    }

  init {
    setBackgroundColor(Color.BLACK)
    isFocusable = false
    isFocusableInTouchMode = false
    descendantFocusability = ViewGroup.FOCUS_BLOCK_DESCENDANTS
    importantForAccessibility = IMPORTANT_FOR_ACCESSIBILITY_NO_HIDE_DESCENDANTS
    stripPlatformViewFocus()
  }

  fun currentSurface(): Surface? {
    if (useTexture) {
      val s = textureSurface
      return if (s != null && s.isValid) s else null
    }
    val s = surfaceView?.holder?.surface
    return if (s != null && s.isValid) s else null
  }

  private var frameAspect = 0.0
  private var frameMode = "default"

  fun setVideoFrame(aspect: Double, mode: String) {
    if (aspect == frameAspect && mode == frameMode) return
    frameAspect = aspect
    frameMode = mode
    layoutVideoFrame()
  }

  private fun layoutVideoFrame() {
    if (surfaceView == null && textureView == null) return
    val w = width
    val h = height
    val (fw, fh) =
      if (frameAspect <= 0.0 || frameMode == "fill" || w <= 0 || h <= 0) {
        LayoutParams.MATCH_PARENT to LayoutParams.MATCH_PARENT
      } else {
        val cover = frameMode == "zoom"
        val byWidth = (frameAspect >= w.toDouble() / h) != cover
        if (byWidth) w to (w / frameAspect).roundToInt() else (h * frameAspect).roundToInt() to h
      }
    for (v in listOfNotNull(surfaceView, textureView, osdSurfaceView)) {
      val cur = v.layoutParams
      if (cur != null && cur.width == fw && cur.height == fh) continue
      v.layoutParams = LayoutParams(fw, fh, Gravity.CENTER)
    }
  }

  override fun onSizeChanged(w: Int, h: Int, oldw: Int, oldh: Int) {
    super.onSizeChanged(w, h, oldw, oldh)
    post { layoutVideoFrame() }
  }

  fun setOsdLayer(enabled: Boolean) {
    osdWanted = enabled
    syncOsdView()
  }

  private fun syncOsdView() {
    val want = osdWanted && surfaceView != null
    if (want == (osdSurfaceView != null)) return
    if (!want) {
      osdReady = false
      onOsdSurface?.invoke()
      removeOsdView()
      return
    }
    val v =
      SurfaceView(context).apply {
        layoutParams = LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT)
        isFocusable = false
        isFocusableInTouchMode = false
        setZOrderMediaOverlay(true)
        holder.setFormat(PixelFormat.TRANSLUCENT)
        holder.addCallback(osdCallback)
      }
    osdSurfaceView = v
    addView(v)
    layoutVideoFrame()
    stripPlatformViewFocus()
  }

  private fun removeOsdView() {
    val v = osdSurfaceView ?: return
    try {
      v.holder.removeCallback(osdCallback)
    } catch (_: Throwable) {
    }
    removeView(v)
    osdSurfaceView = null
    osdReady = false
    osdSize = null
  }

  fun setRender(texture: Boolean) {
    if (useTexture == texture && (surfaceView != null || textureView != null)) {
      if (currentSurface() != null) onSurface?.invoke(true)
      return
    }
    if (surfaceView != null || textureView != null) {
      onSurface?.invoke(false)
    }
    unbindViews()
    removeOsdView()
    listOfNotNull(surfaceView, textureView).forEach { removeView(it) }
    surfaceView = null
    textureView = null
    releaseTextureSurface()
    useTexture = texture
    val lp = LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT)
    if (texture) {
      val tv =
        TextureView(context).apply {
          layoutParams = lp
          isFocusable = false
          isFocusableInTouchMode = false
        }
      textureView = tv
      tv.surfaceTextureListener = this
      addView(tv)
      if (tv.isAvailable) {
        bindTextureSurface(tv.surfaceTexture, tv.width, tv.height)
      }
    } else {
      val sv =
        SurfaceView(context).apply {
          layoutParams = lp
          isFocusable = false
          isFocusableInTouchMode = false
          setZOrderMediaOverlay(true)
          holder.addCallback(this@KotvMpvSurfaceHost)
        }
      surfaceView = sv
      addView(sv)
      layoutVideoFrame()
      syncOsdView()
    }
    stripPlatformViewFocus()
  }

  fun clearVideoLayer() {
    if (surfaceView == null && textureView == null) return
    onSurface?.invoke(false)
    unbindViews()
    removeOsdView()
    listOfNotNull(surfaceView, textureView).forEach { removeView(it) }
    surfaceView = null
    textureView = null
    releaseTextureSurface()
  }

  fun release() {
    unbindViews()
    removeOsdView()
    releaseTextureSurface()
    onSurface = null
    onSurfaceSize = null
    onOsdSurface = null
    removeAllViews()
  }

  private fun unbindViews() {
    try {
      surfaceView?.holder?.removeCallback(this)
    } catch (_: Throwable) {
    }
    textureView?.surfaceTextureListener = null
  }

  private fun releaseTextureSurface() {
    try {
      textureSurface?.release()
    } catch (_: Throwable) {
    }
    textureSurface = null
    bufferSize = null
  }

  private fun bindTextureSurface(st: SurfaceTexture?, width: Int, height: Int) {
    if (st == null) return
    releaseTextureSurface()
    textureSurface = Surface(st)
    applySurfaceSize(width, height)
    onSurface?.invoke(true)
  }

  private fun applySurfaceSize(width: Int, height: Int) {
    if (width <= 0 || height <= 0) return
    bufferSize = "${width}x$height"
    onSurfaceSize?.invoke(width, height)
  }

  override fun surfaceCreated(holder: SurfaceHolder) {
    onSurface?.invoke(true)
  }

  override fun surfaceChanged(holder: SurfaceHolder, format: Int, width: Int, height: Int) {
    applySurfaceSize(width, height)
  }

  override fun surfaceDestroyed(holder: SurfaceHolder) {
    bufferSize = null
    onSurface?.invoke(false)
  }

  override fun onSurfaceTextureAvailable(surface: SurfaceTexture, width: Int, height: Int) {
    bindTextureSurface(surface, width, height)
  }

  override fun onSurfaceTextureSizeChanged(surface: SurfaceTexture, width: Int, height: Int) {
    applySurfaceSize(width, height)
  }

  override fun onSurfaceTextureDestroyed(surface: SurfaceTexture): Boolean {
    onSurface?.invoke(false)
    releaseTextureSurface()
    return true
  }

  override fun onSurfaceTextureUpdated(surface: SurfaceTexture) {}
}
