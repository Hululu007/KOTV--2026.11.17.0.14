import 'dart:async';

import '../api/kotv_api.dart';
import '../player/kotv_traffic.dart';

/// 读健康检查。连不上（端口没开、超时）为 null；
/// 有回应但不是 KOTV 的健康检查（端口被别的程序占着）为空表，[kotvEngineHealthProblem] 会判成不是 KOTV 后端。
Future<Map<String, dynamic>?> kotvEngineHealth(String base) async {
  try {
    final h = await KotvApi(baseUrl: base).health().timeout(const Duration(seconds: 2));
    return Map<String, dynamic>.from(h);
  } on KotvApiException {
    return const <String, dynamic>{};
  } catch (_) {
    return null;
  }
}

/// 端口有回应，但回应的不是 KOTV 引擎。
bool kotvEngineTakenByOther(Map<String, dynamic>? health) => health != null && health['engine'] != 'kotv';

/// 是否同协议的 KOTV 引擎。
Future<bool> kotvEnginePing(String base) async =>
    kotvEngineHealthProblem(await kotvEngineHealth(base)).isEmpty;

KotvApi kotvEngineClient(KotvApi? current, String baseUrl) {
  final api = current ?? KotvApi(baseUrl: baseUrl);
  api.baseUrl = baseUrl;
  KotvTraffic.engineBaseUrl = baseUrl;
  return api;
}
