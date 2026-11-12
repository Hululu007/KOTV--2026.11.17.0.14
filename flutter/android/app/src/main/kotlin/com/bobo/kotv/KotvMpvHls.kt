package com.bobo.kotv

import androidx.media3.common.PlaybackException

/** Media3 MPV：HLS mime 判定与空 mime 时的一次 M3U8 再开（委托 [KotvMedia3Playback]）。 */
internal object KotvMpvHls {
  fun isHlsMime(mime: String?): Boolean = KotvMedia3Playback.isHlsMime(mime)

  fun shouldRetryHlsMime(
    error: PlaybackException,
    currentMime: String?,
    alreadyRetried: Boolean,
  ): Boolean = KotvMedia3Playback.shouldRetryMpvHlsMime(error, currentMime, alreadyRetried)
}
