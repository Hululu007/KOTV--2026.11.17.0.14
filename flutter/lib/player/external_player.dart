import 'package:flutter/services.dart';

import '../api/kotv_api.dart';
import 'kotv_platform.dart';

/// 外部播放器交回的进度。桌面系统打开没有回传，[opened] 仍为真。
class KotvExternalPlayResult {
  const KotvExternalPlayResult({this.opened = false, this.endBy = '', this.positionMs = 0});

  final bool opened;
  final String endBy;
  final int positionMs;
}

const _android = MethodChannel('kotv_android');

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
  // 桌面：系统总能打开 URL（具体应用由 OS 关联决定）。
  avail['view'] = true;
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

/// 安卓：系统应用选择器，并等待对方交回进度。
/// 桌面：引擎按平台系统打开（open / FileProtocolHandler / xdg-open）。
/// 用户关掉安卓选择器时 [KotvExternalPlayResult.opened] 为假。
Future<KotvExternalPlayResult> kotvLaunchExternalPlayer({
  required KotvApi api,
  required String player,
  required String url,
  Map<String, String>? headers,
  String title = '',
  int positionMs = 0,
  bool vod = false,
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
  await api.playerExternal(url: url, player: 'outie#view');
  return const KotvExternalPlayResult(opened: true);
}
