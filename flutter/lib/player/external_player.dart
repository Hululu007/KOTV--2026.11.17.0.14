import 'dart:io' show Platform;

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../api/kotv_api.dart';
import 'kotv_platform.dart';

/// 外部播放器交回的进度。桌面选择器无回传时 [opened] 仍可为真。
class KotvExternalPlayResult {
  const KotvExternalPlayResult({this.opened = false, this.endBy = '', this.positionMs = 0});

  final bool opened;
  final String endBy;
  final int positionMs;
}

const _android = MethodChannel('kotv_android');
const _host = MethodChannel('kotv_host');

/// 把引擎 Available() 与各平台「能否外置打开」并成一张表。
Future<Map<String, dynamic>> _kotvPlayerAvailability(Map? engineAvail) async {
  final avail = Map<String, dynamic>.from(engineAvail ?? const {});
  if (kotvIsAndroid()) {
    var installed = false;
    try {
      installed = await _android.invokeMethod<bool>('hasExternalVideoApp') == true;
    } catch (_) {}
    avail['view'] = installed;
    return avail;
  }
  if (!kIsWeb && (Platform.isMacOS || Platform.isWindows || Platform.isLinux)) {
    try {
      avail['view'] = await _host.invokeMethod<bool>('hasExternalVideoApp') == true;
    } catch (_) {
      avail['view'] = false;
    }
    return avail;
  }
  avail['view'] = false;
  return avail;
}

/// 设置、直播、点播菜单共用：先问引擎，再补各平台外置可用性。
Future<List<(String, String)>> kotvInstalledPlayerOptions({
  required bool live,
  Future<Map<String, dynamic>> Function()? playerStatus,
  Map<String, dynamic>? engineStatus,
}) async {
  Map? engineAvail;
  if (engineStatus != null) {
    engineAvail = engineStatus['available'] as Map?;
  } else if (playerStatus != null) {
    try {
      engineAvail = (await playerStatus())['available'] as Map?;
    } catch (_) {}
  }
  final avail = await _kotvPlayerAvailability(engineAvail);
  final all = live ? kotvLivePlayerOptions() : kotvVodPlayerOptions();
  return kotvVisiblePlayerOptions(all, avail);
}

/// 安卓：系统 Intent 选择器（video/*）。
/// Windows：临时 m3u + 系统「打开方式」。
/// macOS / Linux：列出本机视频应用 → 界面点选 → 用该应用打开 URL。
/// 关掉选择器 / 取消点选时 [KotvExternalPlayResult.opened] 为假。
Future<KotvExternalPlayResult> kotvLaunchExternalPlayer({
  required KotvApi api,
  required String player,
  required String url,
  Map<String, String>? headers,
  String title = '',
  int positionMs = 0,
  bool vod = false,
  BuildContext? context,
}) async {
  if (!kotvIsExternalPlayer(player)) {
    return const KotvExternalPlayResult();
  }
  if (kotvIsAndroid()) {
    final raw = await _android.invokeMethod<dynamic>('openExternalPlayer', {
      'url': url,
      'headers': headers ?? const <String, String>{},
      'title': title,
      'positionMs': positionMs < 0 ? 0 : positionMs,
      'vod': vod,
    });
    if (raw is! Map) return const KotvExternalPlayResult();
    final endBy = '${raw['endBy'] ?? ''}'.trim();
    final pos = int.tryParse('${raw['positionMs'] ?? ''}') ?? 0;
    return KotvExternalPlayResult(
      opened: raw['opened'] == true,
      endBy: endBy,
      positionMs: pos < 0 ? 0 : pos,
    );
  }
  if (kIsWeb) {
    throw StateError('外部播放器仅支持桌面与安卓');
  }
  if (Platform.isWindows) {
    final ok = await _host.invokeMethod<bool>('openExternalVideoChooser', {
      'url': url,
      'title': title,
    });
    return KotvExternalPlayResult(opened: ok == true);
  }
  if (Platform.isMacOS || Platform.isLinux) {
    return _launchDesktopPickedApp(context: context, url: url);
  }
  throw StateError('当前平台不支持外部播放器');
}

Future<KotvExternalPlayResult> _launchDesktopPickedApp({
  required BuildContext? context,
  required String url,
}) async {
  final raw = await _host.invokeMethod<dynamic>('listExternalVideoApps');
  final apps = _parseExternalApps(raw);
  if (apps.isEmpty) {
    throw StateError('未找到可用的外部视频播放器');
  }
  if (context == null || !context.mounted) {
    throw StateError('需要界面以选择外部播放器');
  }
  final picked = await _pickExternalVideoApp(context, apps);
  if (picked == null) return const KotvExternalPlayResult();
  final ok = await _host.invokeMethod<bool>('openExternalVideoApp', {
    'id': picked,
    'url': url,
  });
  return KotvExternalPlayResult(opened: ok == true);
}

List<({String id, String name})> _parseExternalApps(dynamic raw) {
  if (raw is! List) return const [];
  final out = <({String id, String name})>[];
  for (final e in raw) {
    if (e is! Map) continue;
    final id = '${e['id'] ?? ''}'.trim();
    final name = '${e['name'] ?? ''}'.trim();
    if (id.isEmpty) continue;
    out.add((id: id, name: name.isEmpty ? id : name));
  }
  return out;
}

Future<String?> _pickExternalVideoApp(
  BuildContext context,
  List<({String id, String name})> apps,
) {
  return showDialog<String>(
    context: context,
    useRootNavigator: true,
    builder: (ctx) {
      final scheme = Theme.of(ctx).colorScheme;
      return SimpleDialog(
        title: const Text('选择外部播放器'),
        children: [
          for (var i = 0; i < apps.length; i++)
            SimpleDialogOption(
              onPressed: () => Navigator.of(ctx, rootNavigator: true).pop(apps[i].id),
              child: Padding(
                padding: const EdgeInsets.symmetric(vertical: 6),
                child: Text(
                  apps[i].name,
                  style: TextStyle(color: scheme.onSurface),
                ),
              ),
            ),
        ],
      );
    },
  );
}
