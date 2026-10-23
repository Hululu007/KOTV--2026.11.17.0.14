import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:kotv/player/mpv_hls_timeline.dart';

void main() {
  test('extinf playlist is one timeline', () {
    final bytes = '#EXTM3U\n#EXTINF:6,\nhttp://cdn/a.ts\n'.codeUnits;
    expect(kotvMpvHeadIsHlsTimeline(bytes), isTrue);
  });

  test('ext-x tag without extinf is one timeline', () {
    final bytes = '#EXTM3U\n#EXT-X-TARGETDURATION:6\n'.codeUnits;
    expect(kotvMpvHeadIsHlsTimeline(bytes), isTrue);
  });

  test('bare extm3u is not forced', () {
    expect(kotvMpvHeadIsHlsTimeline('#EXTM3U\n'.codeUnits), isFalse);
  });

  test('bom and whitespace still count as a playlist', () {
    final bytes = [0xEF, 0xBB, 0xBF, 0x0A, ...'#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:0\n'.codeUnits];
    expect(kotvMpvHeadIsHlsTimeline(bytes), isTrue);
  });

  test('flv and ts bytes are not forced', () {
    expect(kotvMpvHeadIsHlsTimeline('FLV'.codeUnits), isFalse);
    expect(kotvMpvHeadIsHlsTimeline([0x47, 0x00, 0x11]), isFalse);
  });

  test('non-http url is not read', () async {
    expect(await kotvMpvHlsTimeline('file:///tmp/a.mp4', const {}), isFalse);
    expect(await kotvMpvHlsTimeline('rtsp://cam/live', const {}), isFalse);
  });

  test('forced hls rejection text', () {
    expect(kotvMpvHlsForceRejected('lavf', 'avformat_open_input() failed'), isTrue);
    expect(kotvMpvHlsForceRejected('cplayer', 'Failed to recognize file format.'), isTrue);
    expect(kotvMpvHlsForceRejected('vd', 'could not open codec'), isFalse);
  });

  test('http head uses play headers and stops at the playlist', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    String? ua;
    server.listen((req) {
      ua = req.headers.value('user-agent');
      req.response.write('#EXTM3U\n#EXTINF:4,\nseg.ts\n');
      req.response.close();
    });
    addTearDown(() => server.close(force: true));
    final url = 'http://${server.address.host}:${server.port}/v.m3u8';
    final got = await kotvMpvHlsTimeline(url, {'User-Agent': 'KotvTest'});
    expect(got, isTrue);
    expect(ua, 'KotvTest');
  });

  test('long playlist still counts from the head', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    final body = StringBuffer('#EXTM3U\n#EXT-X-TARGETDURATION:6\n');
    for (var i = 0; i < 4000; i++) {
      body.write('#EXTINF:6,\nhttps://cdn.example/seg$i.ts\n');
    }
    server.listen((req) {
      req.response.write(body.toString());
      req.response.close();
    });
    addTearDown(() => server.close(force: true));
    final url = 'http://${server.address.host}:${server.port}/long.m3u8';
    expect(await kotvMpvHlsTimeline(url, const {}), isTrue);
  });

  test('m3u8 name with flv body is not forced', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    server.listen((req) {
      req.response.add([0x46, 0x4C, 0x56, 0x01]);
      req.response.close();
    });
    addTearDown(() => server.close(force: true));
    final url = 'http://${server.address.host}:${server.port}/clip.m3u8';
    expect(await kotvMpvHlsTimeline(url, const {}), isFalse);
  });

  test('mp4 name is forced only when the body is a playlist', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    server.listen((req) {
      final path = req.uri.path;
      if (path.endsWith('/playlist.mp4')) {
        req.response.write('#EXTM3U\n#EXTINF:6,\nseg.ts\n');
      } else {
        req.response.add([0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70]);
      }
      req.response.close();
    });
    addTearDown(() => server.close(force: true));
    final base = 'http://${server.address.host}:${server.port}';
    expect(await kotvMpvHlsTimeline('$base/playlist.mp4', const {}), isTrue);
    expect(await kotvMpvHlsTimeline('$base/movie.mp4', const {}), isFalse);
  });
}
