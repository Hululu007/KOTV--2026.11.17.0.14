import 'dart:async';

import '../api/kotv_api.dart';
import '../player/kotv_traffic.dart';

/// 读健康检查；连不上或返回的不是 JSON 时为 null。
Future<Map<String, dynamic>?> kotvEngineHealth(String base) async {
  try {
    final h = await KotvApi(baseUrl: base).health().timeout(const Duration(seconds: 2));
    return Map<String, dynamic>.from(h);
  } catch (_) {
    return null;
  }
}

/// 是否同协议的 KOTV 引擎。
Future<bool> kotvEnginePing(String base) async =>
    kotvEngineHealthProblem(await kotvEngineHealth(base)).isEmpty;

KotvApi kotvEngineClient(KotvApi? current, String baseUrl) {
  final api = current ?? KotvApi(baseUrl: baseUrl);
  api.baseUrl = baseUrl;
  KotvTraffic.engineBaseUrl = baseUrl;
  return api;
}
