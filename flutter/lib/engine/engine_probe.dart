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

Future<bool> kotvEnginePing(String base) async {
  final h = await kotvEngineHealth(base);
  return h != null && h['ok'] == true;
}

KotvApi kotvEngineClient(KotvApi? current, String baseUrl) {
  final api = current ?? KotvApi(baseUrl: baseUrl);
  api.baseUrl = baseUrl;
  KotvTraffic.engineBaseUrl = baseUrl;
  return api;
}
