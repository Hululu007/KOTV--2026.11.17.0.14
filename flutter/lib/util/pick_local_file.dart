import 'package:file_selector/file_selector.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../player/kotv_platform.dart';
import '../remote/engine_api.dart';
import '../screens/file_browser_screen.dart';

String? kotvLocalFilePath(String? raw) {
  final text = (raw ?? '').trim();
  if (text.isEmpty) return null;
  if (text.startsWith('file:')) return Uri.parse(text).toFilePath();
  return text;
}

Future<String> _androidStorageRoot() async {
  try {
    const ch = MethodChannel('kotv_android');
    final root = ((await ch.invokeMethod<String>('storageRoot')) ?? '').trim();
    if (root.isNotEmpty) return root;
  } catch (_) {}
  return '/storage/emulated/0';
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
        final root = await _androidStorageRoot();
        if (!context.mounted) return null;
        final picked = await FileBrowserScreen.pick(
          context,
          root: root,
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

/// 选后端主机目录（下载落盘在跑服务的机器上，不是前端沙盒）。
Future<String?> kotvPickEngineDirectory(
  BuildContext context, {
  String title = '选择目录',
}) async {
  if (kIsWeb) return null;
  try {
    final api = await kotvEngineApi();
    if (!context.mounted) return null;
    return kotvLocalFilePath(
      await FileBrowserScreen.pickFolder(
        context,
        root: '',
        title: title,
        listDir: (dir) async {
          final data = await api.fsList(dir);
          final raw = (data['entries'] as List?) ?? const [];
          final items = <KotvFsEntry>[
            for (final e in raw)
              if (e is Map)
                KotvFsEntry(
                  path: '${e['path'] ?? ''}'.trim(),
                  name: '${e['name'] ?? ''}'.trim(),
                  isDir: e['dir'] == true || e['isDir'] == true,
                ),
          ].where((e) => e.path.isNotEmpty && e.name.isNotEmpty).toList();
          return KotvFsListResult(
            items: items,
            parent: '${data['parent'] ?? ''}',
          );
        },
      ),
    );
  } catch (_) {
    return null;
  }
}
