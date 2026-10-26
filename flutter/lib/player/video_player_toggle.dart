import 'package:flutter/foundation.dart';
import 'package:video_player/video_player.dart';

/// video_player 控制器上的播停切换，结束后通知界面。
Future<void> kotvToggleVideoPlayer(VideoPlayerController? c, VoidCallback notify) async {
  if (c == null) return;
  if (c.value.isPlaying) {
    await c.pause();
  } else {
    await c.play();
  }
  notify();
}
