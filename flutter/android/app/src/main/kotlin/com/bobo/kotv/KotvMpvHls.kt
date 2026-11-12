package com.bobo.kotv

import androidx.media3.common.MimeTypes
import androidx.media3.common.PlaybackException

/** Media3 MPV：HLS mime 判定与解析失败后的一次 M3U8 重试条件。 */
internal object KotvMpvHls {
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
   * 与 TV `MpvPlayerEngine.retryHls` 相同：仅 `ERROR_CODE_IO_UNSPECIFIED`，
   * 且当前尚未是 M3U8 mime 时，锁 APPLICATION_M3U8 再开一次。
   */
  fun shouldRetryHlsMime(
    error: PlaybackException,
    currentMime: String?,
    alreadyRetried: Boolean,
  ): Boolean {
    if (alreadyRetried) return false
    if (isHlsMime(currentMime)) return false
    return error.errorCode == PlaybackException.ERROR_CODE_IO_UNSPECIFIED
  }

  fun isDecodeError(error: PlaybackException): Boolean =
    when (error.errorCode) {
      PlaybackException.ERROR_CODE_DECODER_INIT_FAILED,
      PlaybackException.ERROR_CODE_DECODER_QUERY_FAILED,
      PlaybackException.ERROR_CODE_DECODING_FAILED,
      -> true
      else -> false
    }
}
