import 'dart:convert';
import 'dart:io';

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:kotv/api/kotv_api.dart';
import 'package:kotv/api/kotv_engine_url.dart';
import 'package:kotv/engine/engine_probe.dart';

Future<HttpServer> _serve(void Function(HttpResponse res) reply) async {
  final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
  server.listen((req) {
    reply(req.response);
    req.response.close();
  });
  return server;
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  late Directory tmp;

  setUpAll(() async {
    // 测试绑定默认把所有 HTTP 请求都答 400；这里要连真实的本机端口。
    HttpOverrides.global = null;
    tmp = await Directory.systemTemp.createTemp('kotv_probe');
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger.setMockMethodCallHandler(
      const MethodChannel('plugins.flutter.io/path_provider'),
      (_) async => tmp.path,
    );
  });

  tearDownAll(() => tmp.delete(recursive: true));

  test('另一个程序占着端口：有回应但不是 KOTV', () async {
    final other = await _serve((res) => res.statusCode = 200);
    final h = await kotvEngineHealth('http://127.0.0.1:${other.port}');
    await other.close(force: true);
    expect(h, isNotNull);
    expect(kotvEngineTakenByOther(h), isTrue);
    expect(kotvEngineHealthProblem(h), isNotEmpty);
  });

  test('KOTV 引擎：同协议视为可用', () async {
    final kotv = await _serve((res) {
      res.headers.contentType = ContentType.json;
      res.write(jsonEncode({'ok': true, 'engine': 'kotv', 'protocol': kotvProtocolVersion}));
    });
    final h = await kotvEngineHealth('http://127.0.0.1:${kotv.port}');
    await kotv.close(force: true);
    expect(kotvEngineTakenByOther(h), isFalse);
    expect(kotvEngineHealthProblem(h), isEmpty);
  });

  test('端口没开：连不上', () async {
    final closed = await _serve((_) {});
    final port = closed.port;
    await closed.close(force: true);
    final h = await kotvEngineHealth('http://127.0.0.1:$port');
    expect(h, isNull);
    expect(kotvEngineTakenByOther(h), isFalse);
  });
}
