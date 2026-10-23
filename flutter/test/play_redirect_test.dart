import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:kotv/player/play_redirect_io.dart';

void main() {
  test('relative redirect becomes the playlist base and keeps cookies', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    addTearDown(server.close);
    server.listen((req) async {
      if (req.uri.path == '/gw') {
        req.response.statusCode = HttpStatus.found;
        req.response.headers.set(HttpHeaders.locationHeader, '01.m3u8');
        req.response.headers.add(HttpHeaders.setCookieHeader, 'sid=1; Path=/');
        await req.response.close();
        return;
      }
      req.response.statusCode = HttpStatus.ok;
      req.response.write('#EXTM3U');
      await req.response.close();
    });
    final raw = 'http://127.0.0.1:${server.port}/gw';
    final followed = await kotvFollowPlayUrl(raw, {'User-Agent': 'KotvTest'});
    expect(followed.moved, isTrue);
    expect(followed.url, 'http://127.0.0.1:${server.port}/01.m3u8');
    expect(followed.headers['Cookie'], 'sid=1');
    expect(followed.headers['User-Agent'], 'KotvTest');
  });

  test('cross-port redirect drops authorization and keeps an existing cookie', () async {
    final dest = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    addTearDown(dest.close);
    String? auth;
    String? cookie;
    dest.listen((req) async {
      auth = req.headers.value(HttpHeaders.authorizationHeader);
      cookie = req.headers.value(HttpHeaders.cookieHeader);
      req.response.statusCode = HttpStatus.ok;
      await req.response.close();
    });
    final src = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    addTearDown(src.close);
    src.listen((req) async {
      req.response.statusCode = HttpStatus.found;
      req.response.headers.set(
        HttpHeaders.locationHeader,
        'http://127.0.0.1:${dest.port}/file.m3u8',
      );
      req.response.headers.add(HttpHeaders.setCookieHeader, 'hop=2; Path=/');
      await req.response.close();
    });
    final raw = 'http://127.0.0.1:${src.port}/gw';
    final followed = await kotvFollowPlayUrl(raw, {
      'Authorization': 'Bearer secret',
      'Cookie': 'keep=1',
    });
    expect(followed.url, 'http://127.0.0.1:${dest.port}/file.m3u8');
    expect(followed.headers.containsKey('Authorization'), isFalse);
    expect(followed.headers['Cookie'], 'keep=1');
    expect(auth, isNull);
    expect(cookie, 'hop=2');
  });

  test('same host keeps authorization', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    addTearDown(server.close);
    String? auth;
    server.listen((req) async {
      if (req.uri.path == '/gw') {
        req.response.statusCode = HttpStatus.found;
        req.response.headers.set(HttpHeaders.locationHeader, '/file.m3u8');
        await req.response.close();
        return;
      }
      auth = req.headers.value(HttpHeaders.authorizationHeader);
      req.response.statusCode = HttpStatus.ok;
      await req.response.close();
    });
    final raw = 'http://127.0.0.1:${server.port}/gw';
    final followed = await kotvFollowPlayUrl(raw, {'Authorization': 'Bearer secret'});
    expect(followed.url, 'http://127.0.0.1:${server.port}/file.m3u8');
    expect(followed.headers['Authorization'], 'Bearer secret');
    expect(auth, 'Bearer secret');
  });

  test('local proxy and non-http are not requested', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    addTearDown(server.close);
    var hit = false;
    server.listen((req) async {
      hit = true;
      await req.response.close();
    });
    final proxy = 'http://127.0.0.1:${server.port}/proxy/play?u=1';
    final proxied = await kotvFollowPlayUrl(proxy, {});
    expect(proxied.moved, isFalse);
    expect(proxied.url, proxy);
    final file = await kotvFollowPlayUrl('/tmp/a.mkv', {});
    expect(file.moved, isFalse);
    expect(file.url, '/tmp/a.mkv');
    expect(hit, isFalse);
  });

  test('stops after five redirects', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    addTearDown(server.close);
    final seen = <String>[];
    server.listen((req) async {
      seen.add(req.uri.path);
      final n = int.parse(req.uri.path.substring(1));
      req.response.statusCode = HttpStatus.found;
      req.response.headers.set(HttpHeaders.locationHeader, '/${n + 1}');
      await req.response.close();
    });
    final raw = 'http://127.0.0.1:${server.port}/0';
    final followed = await kotvFollowPlayUrl(raw, {});
    expect(followed.url, 'http://127.0.0.1:${server.port}/5');
    expect(seen, ['/0', '/1', '/2', '/3', '/4']);
  });

  test('connection failure keeps the original url', () async {
    final followed = await kotvFollowPlayUrl('http://127.0.0.1:1/gone', {});
    expect(followed.moved, isFalse);
    expect(followed.url, 'http://127.0.0.1:1/gone');
  });
}
