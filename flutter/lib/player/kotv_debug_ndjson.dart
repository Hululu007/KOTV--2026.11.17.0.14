import 'dart:convert';
import 'dart:io';

import 'package:flutter/foundation.dart';

/// Debug-mode NDJSON sink（会话 a027de）。写 TEMP + 尽力 POST ingest。
void kotvDebugNdjson({
  required String hypothesisId,
  required String location,
  required String message,
  Map<String, Object?> data = const {},
  String runId = 'pre',
}) {
  // #region agent log
  try {
    final payload = <String, Object?>{
      'sessionId': 'a027de',
      'runId': runId,
      'hypothesisId': hypothesisId,
      'location': location,
      'message': message,
      'data': data,
      'timestamp': DateTime.now().millisecondsSinceEpoch,
    };
    final line = '${jsonEncode(payload)}\n';
    final tmp = File('${Directory.systemTemp.path}${Platform.pathSeparator}kotv-debug-a027de.log');
    tmp.writeAsStringSync(line, mode: FileMode.append, flush: true);
    // Workspace path（本机/可挂载时）；Win7 发行包上通常不存在，忽略失败。
    try {
      final ws = File('/Users/bobo/Documents/GitHub/KOTV/.cursor/debug-a027de.log');
      if (ws.parent.existsSync()) {
        ws.writeAsStringSync(line, mode: FileMode.append, flush: true);
      }
    } catch (_) {}
    if (!kIsWeb) {
      HttpClient()
          .postUrl(Uri.parse(
              'http://127.0.0.1:7942/ingest/848ba096-86f4-4487-a2c2-83ad4c6218a7'))
          .then((req) {
        req.headers.set('Content-Type', 'application/json');
        req.headers.set('X-Debug-Session-Id', 'a027de');
        req.add(utf8.encode(jsonEncode(payload)));
        return req.close();
      }).then((r) => r.drain<void>()).catchError((_) {});
    }
  } catch (_) {}
  // #endregion
}
