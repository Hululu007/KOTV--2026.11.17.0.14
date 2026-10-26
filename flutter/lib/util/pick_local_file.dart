import 'package:file_selector/file_selector.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../player/kotv_platform.dart';
import '../screens/file_browser_screen.dart';

String? kotvLocalFilePath(String? raw) {
  final text = (raw ?? '').trim();
  if (text.isEmpty) return null;
  if (text.startsWith('file:')) return Uri.parse(text).toFilePath();
  return text;
}

/// Android 用仓内浏览器或系统选择器；桌面用系统对话框；Web 不能选。
Future<String?> kotvPickLocalFile(
  BuildContext context, {
  required String title,
  required List<XTypeGroup> types,
}) async {
  if (kIsWeb) return null;
  if (kotvIsAndroid()) {
    try {
      const ch = MethodChannel('kotv_android');
      try {
        await ch.invokeMethod<bool>('ensureReadMediaPermission');
      } catch (_) {}
      var useBrowser = false;
      try {
        useBrowser = await ch.invokeMethod<bool>('useFileBrowser') ?? false;
      } catch (_) {}
      if (useBrowser && context.mounted) {
        final root = ((await ch.invokeMethod<String>('storageRoot')) ?? '').trim();
        if (!context.mounted) return null;
        final picked = await FileBrowserScreen.pick(
          context,
          root: root.isEmpty ? '/storage/emulated/0' : root,
          title: title,
        );
        return kotvLocalFilePath(picked);
      }
      final path = await ch.invokeMethod<String>('pickConfigFile');
      return kotvLocalFilePath(path);
    } catch (_) {}
  }
  final XFile? file = await openFile(acceptedTypeGroups: types);
  if (file == null) return null;
  return file.path;
}
