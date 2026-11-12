package com.bobo.kotv

import android.net.Uri
import androidx.media3.common.MimeTypes
import androidx.media3.common.PlaybackException
import java.io.File
import java.net.URLDecoder
import java.nio.charset.StandardCharsets

/**
 * Exo / Media3 MPV 共用开播辅助：URI、字幕 mime、错 mime 再开、解码回退、MPV HLS 再开条件。
 * 缓冲字节串见 [mpvDemuxerByteProps]。
 */
internal object KotvMedia3Playback {
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
   * MPV：仅当开播未带 mime（无 format 信号）时，对 IO/容器/清单解析错锁 M3U8 再开一次。
   * 已有非空 mime 时不覆盖（避免把明确点播容器强改成 HLS）。
   */
  fun shouldRetryMpvHlsMime(
    error: PlaybackException,
    currentMime: String?,
    alreadyRetried: Boolean,
  ): Boolean {
    if (alreadyRetried) return false
    if (!currentMime.isNullOrBlank()) return false
    return when (error.errorCode) {
      PlaybackException.ERROR_CODE_IO_UNSPECIFIED,
      PlaybackException.ERROR_CODE_PARSING_CONTAINER_MALFORMED,
      PlaybackException.ERROR_CODE_PARSING_MANIFEST_MALFORMED,
      PlaybackException.ERROR_CODE_PARSING_CONTAINER_UNSUPPORTED,
      PlaybackException.ERROR_CODE_PARSING_MANIFEST_UNSUPPORTED,
      -> true
      else -> false
    }
  }

  fun isDecodeError(error: PlaybackException): Boolean =
    when (error.errorCode) {
      PlaybackException.ERROR_CODE_DECODER_INIT_FAILED,
      PlaybackException.ERROR_CODE_DECODER_QUERY_FAILED,
      PlaybackException.ERROR_CODE_DECODING_FAILED,
      -> true
      else -> false
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
      var mib = (n / (1024 * 1024)).coerceAtLeast(1)
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
