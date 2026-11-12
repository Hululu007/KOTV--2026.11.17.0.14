package com.bobo.kotv

import android.net.Uri
import androidx.media3.common.MimeTypes
import androidx.media3.common.PlaybackException
import java.io.File
import java.net.URLDecoder
import java.nio.charset.StandardCharsets

/** Exo / Media3 MPV 共用：URI、字幕 mime、错误码 → mime、解码回退。 */
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

  /**
   * 错 mime 再开：容器/IO → HLS；清单错 → octet-stream（progressive / 按 URI 再选源）。
   * 与 TV `ExoUtil.getMimeType` 同表。
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
}
