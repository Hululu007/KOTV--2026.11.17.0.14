import 'dart:async';

import 'package:flutter/widgets.dart';

/// 详情和直播共用：切到后台时按设置暂停，或只留声音。
void kotvApplyPlayerBackground(
  AppLifecycleState state, {
  required String mode,
  required bool playing,
  required bool hasMedia,
  required Future<void> Function() pause,
  required Future<void> Function(bool enabled) setVideo,
}) {
  if (state == AppLifecycleState.paused || state == AppLifecycleState.inactive) {
    if (mode == 'off' && playing) {
      unawaited(pause());
    } else if (mode == 'audio' && hasMedia) {
      unawaited(setVideo(false));
    }
  } else if (state == AppLifecycleState.resumed) {
    if (mode == 'audio' && hasMedia) {
      unawaited(setVideo(true));
    }
  }
}
