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

  test('peek skips common media suffixes', () {
    expect(kotvMpvShouldPeekTimeline('https://cdn/a.mp4', null), isFalse);
    expect(kotvMpvShouldPeekTimeline('https://cdn/a.mkv?x=1', null), isFalse);
    expect(kotvMpvShouldPeekTimeline('https://cdn/a.f300.m3u8', null), isTrue);
    expect(kotvMpvShouldPeekTimeline('https://cdn/fengshows?id=2.m3u8', null), isTrue);
    expect(kotvMpvShouldPeekTimeline('http://127.0.0.1:9978/proxy/play?id=1', null), isTrue);
    expect(
      kotvMpvShouldPeekTimeline('https://cdn/a.mp4', 'application/vnd.apple.mpegurl'),
      isTrue,
    );
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
    final got = await kotvMpvHlsTimeline(url, {'User-Agent': 'KotvTest'}, null);
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
    expect(await kotvMpvHlsTimeline(url, const {}, null), isTrue);
  });

  test('m3u8 name with flv body is not forced', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    server.listen((req) {
      req.response.add([0x46, 0x4C, 0x56, 0x01]);
      req.response.close();
    });
    addTearDown(() => server.close(force: true));
    final url = 'http://${server.address.host}:${server.port}/clip.m3u8';
    expect(await kotvMpvHlsTimeline(url, const {}, null), isFalse);
  });
}
