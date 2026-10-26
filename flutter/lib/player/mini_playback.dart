import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';

import '../desktop/mini_player_window.dart';
import 'kotv_playback.dart';

/// 详情和直播共用：没在播就提示；网页走画中画，其它平台走迷你窗。
Future<void> kotvEnterMiniPlayback({
  required BuildContext context,
  required bool Function() isMounted,
  required bool hasMedia,
  required KotvPlayback playback,
  required void Function(VoidCallback fn) setState,
  required void Function(bool inPip) onPip,
  required VoidCallback onEntered,
}) async {
  if (!hasMedia) {
    if (isMounted()) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('请先播放内容')));
    }
    return;
  }
  if (kIsWeb) {
    playback.onPictureInPictureChanged = (inPip) {
      if (!isMounted()) return;
      setState(() => onPip(inPip));
    };
    final ok = await playback.enterPictureInPicture();
    if (!isMounted() || !context.mounted) return;
    if (ok) {
      setState(onEntered);
    } else {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('无法进入画中画，请检查浏览器是否支持')),
      );
    }
    return;
  }
  if (!MiniPlayerWindow.supported) {
    if (isMounted()) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('当前平台不支持迷你桌面播放')));
    }
    return;
  }
  MiniPlayerWindow.onAndroidPipChanged = (inPip) {
    if (!isMounted()) return;
    setState(() => onPip(inPip));
  };
  await MiniPlayerWindow.enter();
  if (!isMounted() || !context.mounted) return;
  if (MiniPlayerWindow.active) {
    setState(onEntered);
  } else if (isMounted()) {
    ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('无法进入小窗，请检查系统是否支持画中画')));
  }
}

Future<void> kotvExitMiniPlayback({
  required bool Function() isMounted,
  required KotvPlayback playback,
  required void Function(VoidCallback fn) setState,
  required VoidCallback onExited,
}) async {
  if (kIsWeb) {
    await playback.exitPictureInPicture();
    if (!isMounted()) return;
    setState(onExited);
    return;
  }
  await MiniPlayerWindow.exit();
  if (!isMounted()) return;
  setState(onExited);
}
