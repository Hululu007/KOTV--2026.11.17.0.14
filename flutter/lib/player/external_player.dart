import 'package:flutter/services.dart';

import '../api/kotv_api.dart';
import 'kotv_platform.dart';

/// 外部播放器交回的进度。桌面进程启动没有回传，[opened] 仍为真。
class KotvExternalPlayResult {
  const KotvExternalPlayResult({this.opened = false, this.endBy = '', this.positionMs = 0});

  final bool opened;
  final String endBy;
  final int positionMs;
}

const _android = MethodChannel('kotv_android');

/// 把引擎 Available() 和安卓「还有别的视频应用」并成一张表。
Future<Map<String, dynamic>> _kotvPlayerAvailability(Map? engineAvail) async {
  final avail = Map<String, dynamic>.from(engineAvail ?? const {});
  if (!kotvIsAndroid()) return avail;
  var installed = false;
  try {
    installed = await _android.invokeMethod<bool>('hasExternalVideoApp') == true;
  } catch (_) {}
  avail['view'] = installed;
  return avail;
}

/// 设置、直播、点播菜单共用：先问引擎，安卓再问本机还有没有别的视频应用。
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

/// 桌面走引擎进程。安卓走系统选择器，并等待对方交回进度。
/// 用户关掉选择器时 [KotvExternalPlayResult.opened] 为假。
Future<KotvExternalPlayResult> kotvLaunchExternalPlayer({
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
    if (raw is! Map) return const KotvExternalPlayResult();
    final endBy = '${raw['endBy'] ?? ''}'.trim();
    final pos = int.tryParse('${raw['positionMs'] ?? ''}') ?? 0;
    return KotvExternalPlayResult(
      opened: raw['opened'] == true,
      endBy: endBy,
      positionMs: pos < 0 ? 0 : pos,
    );
  }
  await api.playerExternal(url: url, player: name);
  return const KotvExternalPlayResult(opened: true);
}
