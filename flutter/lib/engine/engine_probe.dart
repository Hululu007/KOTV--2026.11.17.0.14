import 'dart:async';

import '../api/kotv_api.dart';
import '../player/kotv_traffic.dart';

Future<Map<String, dynamic>?> kotvEngineHealth(String base) async {
  try {
    final h = await KotvApi(baseUrl: base).health().timeout(const Duration(seconds: 2));
    return Map<String, dynamic>.from(h);
  } catch (_) {
    return null;
  }
}

/// 是否 KOTV 引擎的健康响应；端口上是别的服务时为 false。
bool kotvIsEngineHealth(Map<String, dynamic>? h) =>
    h != null && h['ok'] == true && h['engine'] == 'kotv';

Future<bool> kotvEnginePing(String base) async => kotvIsEngineHealth(await kotvEngineHealth(base));

/// 探测用户填写的后端地址：连不上或不是 KOTV 后端直接抛错，否则返回健康响应。
Future<Map<String, dynamic>> kotvProbeEngine(String base) async {
  final h = Map<String, dynamic>.from(
      await KotvApi(baseUrl: base).health().timeout(const Duration(seconds: 6)));
  if (!kotvIsEngineHealth(h)) throw Exception('该地址不是 KOTV 后端');
  return h;
}

KotvApi kotvEngineClient(KotvApi? current, String baseUrl) {
  final api = current ?? KotvApi(baseUrl: baseUrl);
  api.baseUrl = baseUrl;
  KotvTraffic.engineBaseUrl = baseUrl;
  return api;
}
