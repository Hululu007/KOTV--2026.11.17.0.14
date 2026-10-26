import 'dart:js_interop';

import 'package:flutter/foundation.dart';
import 'package:web/web.dart' as web;

/// 网页播放器进入画中画。调用方先确认元素存在、监听已挂上。
Future<bool> kotvEnterWebPictureInPicture({
  required web.HTMLVideoElement video,
  required void Function(bool active) setActive,
  required void Function(bool active)? onChanged,
  required VoidCallback notify,
}) async {
  try {
    if (web.document.pictureInPictureElement == video) {
      setActive(true);
      return true;
    }
    await video.requestPictureInPicture().toDart;
    setActive(true);
    onChanged?.call(true);
    notify();
    return true;
  } catch (_) {
    return false;
  }
}

/// 网页播放器退出画中画。已经不在画中画里时只清本地状态。
Future<void> kotvExitWebPictureInPicture({
  required void Function(bool active) setActive,
  required void Function(bool active)? onChanged,
  required VoidCallback notify,
}) async {
  try {
    if (web.document.pictureInPictureElement != null) {
      await web.document.exitPictureInPicture().toDart;
    }
  } catch (_) {}
  setActive(false);
  onChanged?.call(false);
  notify();
}
