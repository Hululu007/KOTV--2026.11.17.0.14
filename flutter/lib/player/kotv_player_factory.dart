import 'dart:async';

import 'package:flutter/material.dart';
import 'package:media_kit_video/media_kit_video.dart';

import 'art_playback.dart';
import 'exo_playback.dart';
import 'html_playback.dart';
import 'kotv_playback.dart';
import 'kotv_platform.dart';
import 'media_kit_playback.dart';
import 'mpv_opts.dart';
import 'native_mpv_playback.dart';
import 'subtitle_style_util.dart';
import 'xg_playback.dart';
import 'zw_playback.dart';

/// 把播放设置推给页面持有的后端：MPV 选项、Exo 选项，再给当前后端写字幕样式与偏移。
/// 点播与直播页共用。
void kotvPushPlayerSettings(
  Map<String, dynamic> settings, {
  required KotvMpvOpts mpvOpts,
  required KotvPlayback current,
  KotvPlayback? mpv,
  ExoPlayback? exo,
}) {
  if (mpv is NativeMpvPlayback) {
    unawaited(mpv.applyOpts(mpvOpts));
  } else if (mpv is MediaKitPlayback) {
    unawaited(mpv.applyOpts(mpvOpts));
  }
  exo?.applyPlayerOptions(settings);
  // Exo 的 applyPlayerOptions 播放中已按同一份设置写过字幕样式与偏移。
  if (!identical(current, exo)) unawaited(kotvApplySubtitleSettings(current, settings));
}

/// 播放菜单是否显示「硬解直出」：当前后端有这个开关，且没选软解。
bool kotvShowDirectOutput(KotvPlayback playback, String decodeMode) =>
    playback.supportsDirectOutput && !KotvMpvOpts(decodeMode: decodeMode).soft;

/// 页内画面：Android 原生 MPV PlatformView；桌面/Windows/iOS media_kit Video。
Widget kotvPlaybackView({
  required String playerVal,
  required KotvPlayback playback,
  KotvPlayback? mpv,
  BoxFit fit = BoxFit.contain,
}) {
  switch (kotvEmbedBackend(playerVal)) {
    case KotvEmbedBackend.html:
      if (playback is HtmlPlayback) return playback.buildView(fit: fit);
      return const ColoredBox(color: Colors.black);
    case KotvEmbedBackend.art:
      if (playback is ArtPlayback) return playback.buildView(fit: fit);
      return const ColoredBox(color: Colors.black);
    case KotvEmbedBackend.xg:
      if (playback is XgPlayback) return playback.buildView(fit: fit);
      return const ColoredBox(color: Colors.black);
    case KotvEmbedBackend.zw:
      if (playback is ZwPlayback) return playback.buildView(fit: fit);
      return const ColoredBox(color: Colors.black);
    case KotvEmbedBackend.exo:
      if (playback is ExoPlayback) return playback.buildView(fit: fit);
      return const ColoredBox(color: Colors.black);
    case KotvEmbedBackend.mpv:
      final m = mpv ?? playback;
      if (m is NativeMpvPlayback) return m.buildView(fit: fit);
      if (m is MediaKitPlayback) {
        // ValueKey：换集/重建 Player 时 remount，避免挂已 dispose 的 Texture。
        return KeyedSubtree(
          key: ValueKey<int>(identityHashCode(m.player)),
          child: Video(
            controller: m.controller,
            controls: NoVideoControls,
            fit: fit,
            wakelock: false,
            // 桌面窗失焦/最小化也会进 paused；默认 true 会 pause 掉 libmpv，易黑屏。
            pauseUponEnteringBackgroundMode: kotvIsAndroid(),
          ),
        );
      }
      return const ColoredBox(color: Colors.black);
  }
}
