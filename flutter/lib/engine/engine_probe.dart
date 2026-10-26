import 'dart:async';

import '../api/kotv_api.dart';

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
