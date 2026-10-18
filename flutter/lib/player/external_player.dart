import 'package:flutter/services.dart';

import '../api/kotv_api.dart';
import 'kotv_platform.dart';

/// 外部播放器交回的进度。桌面进程启动没有回传，返回 null。
class KotvExternalPlayResult {
  const KotvExternalPlayResult({this.endBy = '', this.positionMs = 0});

  final String endBy;
  final int positionMs;
}

const _android = MethodChannel('kotv_android');

/// 把引擎 Available() 和安卓「还有别的视频应用」并成一张表。
Future<Map<String, dynamic>> kotvPlayerAvailability(Map? engineAvail) async {
  final avail = Map<String, dynamic>.from(engineAvail ?? const {});
  if (kotvIsAndroid()) {
    avail['view'] = await kotvAndroidHasExternalPlayer();
  }
  return avail;
}

Future<bool> kotvAndroidHasExternalPlayer() async {
  if (!kotvIsAndroid()) return false;
  try {
    final ok = await _android.invokeMethod<bool>('hasExternalVideoApp');
    return ok == true;
  } catch (_) {
    return false;
  }
}

/// 桌面走引擎进程。安卓走系统选择器，并等待对方交回进度。
Future<KotvExternalPlayResult?> kotvLaunchExternalPlayer({
  required KotvApi api,
  required String player,
  required String url,
  Map<String, String>? headers,
  String title = '',
  int positionMs = 0,
  bool vod = false,
}) async {
  final name = player.trim();
  if (name == 'outie#view') {
    if (!kotvIsAndroid()) {
      throw StateError('外部播放器仅支持安卓');
    }
    final raw = await _android.invokeMethod<dynamic>('openExternalPlayer', {
      'url': url,
      'headers': headers ?? const <String, String>{},
      'title': title,
      'positionMs': positionMs < 0 ? 0 : positionMs,
      'vod': vod,
    });
    if (raw is! Map) return null;
    final endBy = '${raw['endBy'] ?? ''}'.trim();
    final pos = int.tryParse('${raw['positionMs'] ?? ''}') ?? 0;
    return KotvExternalPlayResult(endBy: endBy, positionMs: pos < 0 ? 0 : pos);
  }
  await api.playerExternal(url: url, player: name);
  return null;
}
