package com.bobo.kotv

import android.net.Uri
import androidx.media3.common.MimeTypes
import androidx.media3.common.PlaybackException
import java.io.File
import java.net.URLDecoder
import java.nio.charset.StandardCharsets

/**
 * 安卓 Exo / 原生 MPV 共用开播辅助。
 *
 * Exo：URI、字幕 mime、错 mime 再开、IO 错码。
 * MPV：HLS lavf（常量与 Dart [kotvDemuxerLavfO] / [kotvApplyLavfForOpen] 同源）、
 * [mpvDemuxerByteProps]、解码回退、文案重试。改 lavf 串须同步 Dart。
 */
internal object KotvAndroidPlayback {
  /** Dart [kotvDemuxerLavfO]。 */
  const val LAVF_DEMUXER_O =
    "seg_max_retry=5,strict=experimental," +
      "allowed_extensions=ALL,allowed_segment_extensions=ALL,extension_picky=0," +
      "probesize=8000000,analyzeduration=8000000"

  /** Dart [kotvHlsLavfOAddForUri]。 */
  const val HLS_LAVF_O_ADD_URI = "http_persistent=0"

  /** Dart [kotvHlsLavfOAddForMime]。 */
  const val HLS_LAVF_O_ADD_MIME = "extension_picky=0,http_persistent=0"
  fun playUri(url: String): Uri {
    val t = url.trim()
    val low = t.lowercase()
    if (low.startsWith("content:")) return Uri.parse(t)
    if (t.startsWith("/")) return Uri.fromFile(File(t))
    if (low.startsWith("file:")) {
      try {
        val parsed = Uri.parse(t)
        var path = parsed.path
        if (!path.isNullOrEmpty()) {
          path = URLDecoder.decode(path, StandardCharsets.UTF_8.name())
          return Uri.fromFile(File(path))
        }
      } catch (_: Throwable) {
      }
      return Uri.parse(t)
    }
    return Uri.parse(t)
  }

  fun subtitleMimeFromPath(url: String): String {
    val path =
      try {
        Uri.parse(url).path ?: url
      } catch (_: Throwable) {
        url
      }.lowercase()
    return when {
      path.endsWith(".vtt") -> MimeTypes.TEXT_VTT
      path.endsWith(".ssa") || path.endsWith(".ass") -> MimeTypes.TEXT_SSA
      path.endsWith(".ttml") || path.endsWith(".xml") || path.endsWith(".dfxp") -> MimeTypes.APPLICATION_TTML
      else -> MimeTypes.APPLICATION_SUBRIP
    }
  }

  fun isHlsMime(mime: String?): Boolean {
    val m = mime?.trim().orEmpty()
    if (m.isEmpty()) return false
    return m.equals(MimeTypes.APPLICATION_M3U8, ignoreCase = true) ||
      m.equals("application/vnd.apple.mpegurl", ignoreCase = true) ||
      m.equals("audio/mpegurl", ignoreCase = true) ||
      m.equals("audio/x-mpegurl", ignoreCase = true) ||
      m.equals("application/x-mpegURL", ignoreCase = true)
  }

  /** 与 Dart [kotvFormatIsHls] 一致（含 `hls` / `m3u8` 短名）。 */
  fun formatIsHls(format: String?): Boolean {
    val f = format?.trim()?.lowercase().orEmpty()
    if (f.isEmpty()) return false
    if (isHlsMime(format)) return true
    return f == "hls" || f == "m3u8"
  }

  /** 与 Dart [kotvUrlIsHls] 一致的 URL 强信号。 */
  fun urlIsHls(url: String): Boolean {
    val raw = url.trim()
    if (raw.isEmpty()) return false
    val uri =
      try {
        Uri.parse(raw)
      } catch (_: Throwable) {
        return raw.lowercase().contains("=m3u8")
      }
    val scheme = uri.scheme?.lowercase().orEmpty()
    if (scheme == "rtsp" || scheme == "rtspt") return false
    if (scheme == "data") {
      val body = if (raw.length > 5) raw.substring(5) else ""
      val mime = body.substringBefore(',').lowercase().trim()
      return mime.contains("mpegurl") || mime.contains("m3u8")
    }
    val lower = raw.lowercase()
    if (lower.contains("=m3u8")) return true
    if (lower.contains("=mpd")) return false
    for (seg in uri.pathSegments) {
      if (seg.isNullOrEmpty()) continue
      if (seg.lowercase().contains("m3u8")) return true
    }
    return false
  }

  /** 与 Dart [kotvIsHls]：URL 强信号或 mime/format 表示 HLS。 */
  fun isHls(url: String, mimeOrFormat: String?): Boolean =
    urlIsHls(url) || formatIsHls(mimeOrFormat)

  /**
   * 与 Dart [kotvApplyLavfForOpen] 等价：仅 HLS 时写入 loadfile 的
   * `demuxer-lavf-format=hls` + `demuxer-lavf-o-add=…`。
   */
  fun appendLavfHlsLoadOpts(url: String, mimeOrFormat: String?, opts: MutableList<String>) {
    if (!isHls(url, mimeOrFormat)) return
    opts.add("demuxer-lavf-format=hls")
    val add = if (urlIsHls(url)) HLS_LAVF_O_ADD_URI else HLS_LAVF_O_ADD_MIME
    opts.add("demuxer-lavf-o-add=$add")
  }

  /**
   * Exo：容器/IO → HLS；清单错 → octet-stream（progressive / 按 URI 再选源）。
   */
  fun mimeForError(errorCode: Int): String? =
    when (errorCode) {
      PlaybackException.ERROR_CODE_PARSING_CONTAINER_UNSUPPORTED,
      PlaybackException.ERROR_CODE_PARSING_CONTAINER_MALFORMED,
      PlaybackException.ERROR_CODE_IO_UNSPECIFIED,
      -> MimeTypes.APPLICATION_M3U8
      PlaybackException.ERROR_CODE_PARSING_MANIFEST_UNSUPPORTED,
      PlaybackException.ERROR_CODE_PARSING_MANIFEST_MALFORMED,
      -> MimeTypes.APPLICATION_OCTET_STREAM
      else -> null
    }

  /**
   * 原生 MPV：Dart [kotvShouldRetryMpvHlsAfterError]。
   * 已按 URL/mime 当 HLS 开过则不再试；仅 IO/打开失败文案触发。
   */
  fun shouldRetryMpvHlsAfterError(
    errorText: String,
    url: String,
    currentMime: String?,
    alreadyRetried: Boolean,
  ): Boolean {
    if (alreadyRetried) return false
    if (isHls(url, currentMime)) return false
    val t = errorText.lowercase()
    if (t.contains("tcp:") || (t.contains("http") && t.contains("error"))) return true
    if (t.contains("failed to open") || t.contains("error opening") || t.contains("i/o")) {
      return true
    }
    return false
  }

  /** Exo 解码类 PlaybackException。 */
  fun isDecodeError(error: PlaybackException): Boolean =
    when (error.errorCode) {
      PlaybackException.ERROR_CODE_DECODER_INIT_FAILED,
      PlaybackException.ERROR_CODE_DECODER_QUERY_FAILED,
      PlaybackException.ERROR_CODE_DECODING_FAILED,
      -> true
      else -> false
    }

  /** 原生 MPV `end-file` 错误文案是否像解码/hwdec 失败。 */
  fun isDecodeErrorText(errorText: String): Boolean {
    val t = errorText.lowercase()
    return t.contains("decoder") ||
      t.contains("hwdec") ||
      t.contains("could not open codec") ||
      t.contains("error initializing") ||
      t.contains("no decoder") ||
      t.contains("codec not supported")
  }

  /** soft ↔ hard；auto 先试 soft。 */
  fun nextDecodeMode(current: String): String =
    when (KotvPlayerNormalize.decodeModeLabel(current)) {
      "soft" -> "hard"
      "hard" -> "soft"
      else -> "soft"
    }

  /** 对应 Dart [KotvBufferBudget.mpvCacheProps]；[budgetBytes]≤0 时返回空。 */
  fun mpvDemuxerByteProps(budgetBytes: Int): Map<String, String> {
    if (budgetBytes <= 0) return emptyMap()
    fun miB(n: Int): String {
      val mib = (n / (1024 * 1024)).coerceAtLeast(1)
      return "${mib}MiB"
    }
    return mapOf(
      "cache" to "yes",
      "demuxer-max-bytes" to miB(budgetBytes),
      "demuxer-max-back-bytes" to miB(budgetBytes / 8),
      "cache-pause-initial" to "no",
      "framedrop" to "vo",
    )
  }
}
