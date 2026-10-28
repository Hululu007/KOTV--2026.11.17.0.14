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

/// 探测后端地址：连不上、不是 KOTV 后端或版本不兼容时抛错并说明原因，否则返回健康响应。
Future<Map<String, dynamic>> kotvProbeEngine(String base) async {
  final Map<String, dynamic> h;
  try {
    h = Map<String, dynamic>.from(
        await KotvApi(baseUrl: base).health().timeout(const Duration(seconds: 6)));
  } on KotvApiException {
    throw KotvApiException('该地址不是 KOTV 后端');
  } catch (_) {
    throw KotvApiException('无法连接 $base');
  }
  final problem = kotvEngineHealthProblem(h);
  if (problem.isNotEmpty) throw KotvApiException(problem);
  return h;
}

KotvApi kotvEngineClient(KotvApi? current, String baseUrl) {
  final api = current ?? KotvApi(baseUrl: baseUrl);
  api.baseUrl = baseUrl;
  KotvTraffic.engineBaseUrl = baseUrl;
  return api;
}
