import 'dart:io' show Platform;

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'kotv_platform.dart';

/// 外部播放器交回的进度。桌面选择器无回传时 [opened] 仍可为真。
class KotvExternalPlayResult {
  const KotvExternalPlayResult({this.opened = false, this.endBy = '', this.positionMs = 0});

  final bool opened;
  final String endBy;
  final int positionMs;
}

class _ExternalApp {
  const _ExternalApp({required this.id, required this.name, this.icon});

  final String id;
  final String name;
  final Uint8List? icon;
}

const _android = MethodChannel('kotv_android');
const _host = MethodChannel('kotv_host');

/// 引擎 Available() + 各平台「能否外置打开」并成一张表。
Future<Map<String, dynamic>> _kotvPlayerAvailability(Map? engineAvail) async {
  final avail = Map<String, dynamic>.from(engineAvail ?? const {});
  avail['view'] = await _platformHasExternalVideoApp();
  return avail;
}

Future<bool> _platformHasExternalVideoApp() async {
  if (kotvIsAndroid()) {
    try {
      return await _android.invokeMethod<bool>('hasExternalVideoApp') == true;
    } catch (_) {
      return false;
    }
  }
  if (!kIsWeb && (Platform.isMacOS || Platform.isWindows || Platform.isLinux)) {
    try {
      return await _host.invokeMethod<bool>('hasExternalVideoApp') == true;
    } catch (_) {
      return false;
    }
  }
  return false;
}

/// 设置、直播、点播菜单共用。
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
  // 别名目前指向同一表；保留 live 分支便于日后点播/直播选项分叉。
  final all = live ? kotvLivePlayerOptions() : kotvVodPlayerOptions();
  return kotvVisiblePlayerOptions(all, avail);
}

/// 打开外部播放器：
/// - 安卓：系统 Intent 选择器（video/*），地址即当前播放地址（含代理）
/// - 桌面：宿主列出本机视频应用（含可选 64px PNG `icon`）→ 网格点选 → 原址交给该应用
/// 取消选择时 [KotvExternalPlayResult.opened] 为假；外部播不了也不再改写地址。
Future<KotvExternalPlayResult> kotvLaunchExternalPlayer({
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
    return _launchAndroid(
      url: url,
      headers: headers,
      title: title,
      positionMs: positionMs,
      vod: vod,
    );
  }
  if (kIsWeb) {
    throw StateError('外部播放器仅支持桌面与安卓');
  }
  if (Platform.isWindows || Platform.isMacOS || Platform.isLinux) {
    return _launchDesktopPickedApp(context: context, url: url);
  }
  throw StateError('当前平台不支持外部播放器');
}

Future<KotvExternalPlayResult> _launchAndroid({
  required String url,
  Map<String, String>? headers,
  required String title,
  required int positionMs,
  required bool vod,
}) async {
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

Future<KotvExternalPlayResult> _launchDesktopPickedApp({
  required BuildContext? context,
  required String url,
}) async {
  final apps = _parseExternalApps(await _host.invokeMethod<dynamic>('listExternalVideoApps'));
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

List<_ExternalApp> _parseExternalApps(dynamic raw) {
  if (raw is! List) return const [];
  final out = <_ExternalApp>[];
  for (final e in raw) {
    if (e is! Map) continue;
    final id = '${e['id'] ?? ''}'.trim();
    final name = '${e['name'] ?? ''}'.trim();
    if (id.isEmpty) continue;
    out.add(_ExternalApp(
      id: id,
      name: name.isEmpty ? id : name,
      icon: _asIconBytes(e['icon']),
    ));
  }
  return out;
}

Uint8List? _asIconBytes(dynamic raw) {
  if (raw is Uint8List) return raw.isEmpty ? null : raw;
  if (raw is ByteData) {
    final view = raw.buffer.asUint8List(raw.offsetInBytes, raw.lengthInBytes);
    return view.isEmpty ? null : view;
  }
  if (raw is List) {
    if (raw.isEmpty) return null;
    return Uint8List.fromList(raw.cast<int>());
  }
  return null;
}

Future<String?> _pickExternalVideoApp(BuildContext context, List<_ExternalApp> apps) {
  return showDialog<String>(
    context: context,
    useRootNavigator: true,
    builder: (ctx) {
      final scheme = Theme.of(ctx).colorScheme;
      final wide = MediaQuery.sizeOf(ctx).width;
      final cols = wide >= 720 ? 4 : (wide >= 520 ? 3 : 2);
      final maxH = MediaQuery.sizeOf(ctx).height * 0.55;
      return AlertDialog(
        title: const Text('选择外部播放器'),
        content: SizedBox(
          width: cols * 108.0,
          child: ConstrainedBox(
            constraints: BoxConstraints(maxHeight: maxH),
            child: GridView.builder(
              shrinkWrap: true,
              gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
                crossAxisCount: cols,
                mainAxisSpacing: 8,
                crossAxisSpacing: 8,
                childAspectRatio: 0.82,
              ),
              itemCount: apps.length,
              itemBuilder: (_, i) {
                final app = apps[i];
                return InkWell(
                  borderRadius: BorderRadius.circular(12),
                  onTap: () => Navigator.of(ctx, rootNavigator: true).pop(app.id),
                  child: Padding(
                    padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 8),
                    child: Column(
                      mainAxisAlignment: MainAxisAlignment.center,
                      children: [
                        _ExternalAppIcon(bytes: app.icon, name: app.name, scheme: scheme),
                        const SizedBox(height: 8),
                        Text(
                          app.name,
                          maxLines: 2,
                          overflow: TextOverflow.ellipsis,
                          textAlign: TextAlign.center,
                          style: TextStyle(fontSize: 12, color: scheme.onSurface),
                        ),
                      ],
                    ),
                  ),
                );
              },
            ),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx, rootNavigator: true).pop(),
            child: const Text('取消'),
          ),
        ],
      );
    },
  );
}

class _ExternalAppIcon extends StatelessWidget {
  const _ExternalAppIcon({required this.bytes, required this.name, required this.scheme});

  final Uint8List? bytes;
  final String name;
  final ColorScheme scheme;

  @override
  Widget build(BuildContext context) {
    final data = bytes;
    if (data != null && data.isNotEmpty) {
      return ClipRRect(
        borderRadius: BorderRadius.circular(12),
        child: Image.memory(
          data,
          width: 56,
          height: 56,
          filterQuality: FilterQuality.medium,
          gaplessPlayback: true,
          errorBuilder: (_, __, ___) => _fallback(),
        ),
      );
    }
    return _fallback();
  }

  Widget _fallback() {
    final label = name.trim().isEmpty ? '?' : String.fromCharCodes(name.runes.take(1));
    return Container(
      width: 56,
      height: 56,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        // Win7 / Flutter 3.19 无 surfaceContainerHighest。
        color: Color.lerp(scheme.surface, scheme.onSurface, 0.12) ?? scheme.surface,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Text(
        label,
        style: TextStyle(fontSize: 22, fontWeight: FontWeight.w600, color: scheme.onSurface),
      ),
    );
  }
}
