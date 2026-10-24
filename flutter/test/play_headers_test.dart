import 'package:flutter_test/flutter_test.dart';
import 'package:kotv/player/kotv_platform.dart';
import 'package:kotv/player/play_headers.dart';

void main() {
  test('header-capable player opens the site url', () {
    final got = kotvResolvePlayOpenTarget(
      playUrl: 'http://127.0.0.1:9978/proxy/play?id=1',
      directUrl: 'https://cdn.example/live.m3u8',
      headers: const {'Cookie': 'a=b'},
      magnet: false,
      sendHeaders: true,
    );
    expect(got.url, 'https://cdn.example/live.m3u8');
    expect(got.headers?['Cookie'], 'a=b');
  });

  test('other players keep the engine url', () {
    final got = kotvResolvePlayOpenTarget(
      playUrl: 'http://127.0.0.1:9978/proxy/play?id=1',
      directUrl: 'https://cdn.example/live.m3u8',
      headers: const {'Cookie': 'a=b'},
      magnet: false,
    );
    expect(got.url, contains('/proxy/play'));
    expect(got.headers, isNull);
  });

  test('loopback site url stays on the engine url', () {
    final got = kotvResolvePlayOpenTarget(
      playUrl: 'http://127.0.0.1:9978/proxy/play?id=1',
      directUrl: 'http://127.0.0.1:5266/a.mp4',
      headers: const {'Cookie': 'a=b'},
      magnet: false,
      sendHeaders: true,
    );
    expect(got.url, contains('/proxy/play'));
    expect(got.headers, isNull);
  });

  test('hls.js html player opens the site url', () {
    expect(
      kotvPlaybackSendsHeaders(
        playerVal: 'innie#html',
        mediaUrl: 'https://cdn.example/live.m3u8',
        hlsJsSupported: true,
      ),
      isTrue,
    );
    final got = kotvResolvePlayOpenTarget(
      playUrl: 'http://127.0.0.1:9978/proxy/play?id=1',
      directUrl: 'https://cdn.example/live.m3u8',
      headers: const {'Cookie': 'a=b'},
      magnet: false,
      sendHeaders: true,
    );
    expect(got.url, 'https://cdn.example/live.m3u8');
    expect(got.headers?['Cookie'], 'a=b');
  });

  test('fvp exo mpv always; web hls.js only on playlists', () {
    expect(
      kotvPlaybackSendsHeaders(
        playerVal: 'innie#fvp',
        mediaUrl: 'https://cdn.example/a.mp4',
      ),
      isTrue,
    );
    expect(
      kotvPlaybackSendsHeaders(
        playerVal: 'innie#exo',
        mediaUrl: 'https://cdn.example/live.m3u8',
      ),
      isTrue,
    );
    expect(
      kotvPlaybackSendsHeaders(
        playerVal: 'innie#mpv',
        mediaUrl: 'https://cdn.example/live.m3u8',
      ),
      isTrue,
    );
    expect(
      kotvPlaybackSendsHeaders(
        playerVal: 'innie#html',
        mediaUrl: 'https://cdn.example/live.m3u8',
        hlsJsSupported: false,
      ),
      isFalse,
    );
    expect(
      kotvPlaybackSendsHeaders(
        playerVal: 'innie#art',
        mediaUrl: 'https://cdn.example/live.m3u8?x=1',
        hlsJsSupported: true,
      ),
      isTrue,
    );
    expect(
      kotvPlaybackSendsHeaders(
        playerVal: 'innie#art',
        mediaUrl: 'https://cdn.example/a.mp4',
        hlsJsSupported: true,
      ),
      isFalse,
    );
    expect(
      kotvPlaybackSendsHeaders(
        playerVal: 'innie#xg',
        mediaUrl: 'https://cdn.example/live.m3u8',
        hlsJsSupported: true,
      ),
      isTrue,
    );
    expect(
      kotvPlaybackSendsHeaders(
        playerVal: 'innie#xg',
        mediaUrl: 'https://cdn.example/a.mp4',
        hlsJsSupported: true,
      ),
      isFalse,
    );
    expect(
      kotvPlaybackSendsHeaders(
        playerVal: 'innie#zw',
        mediaUrl: 'https://cdn.example/live.m3u8',
        hlsJsSupported: true,
      ),
      isTrue,
    );
    expect(
      kotvPlaybackSendsHeaders(
        playerVal: 'innie#html',
        mediaUrl: 'https://cdn.example/a.mp4',
        format: 'application/x-mpegURL',
        hlsJsSupported: true,
      ),
      isTrue,
    );
    expect(
      kotvPlaybackSendsHeaders(
        playerVal: 'innie#html',
        mediaUrl: 'https://cdn.example/a.mp4',
        format: 'm3u8',
        hlsJsSupported: true,
      ),
      isFalse,
    );
  });

  test('mpv keeps ua and referer off the header field list', () {
    final split = kotvSplitMpvHeaders(const {
      'User-Agent': 'Site/1',
      'Referer': 'https://site.example/',
      'Cookie': 'a=b',
      'Origin': 'https://site.example',
      'Range': 'bytes=0-1',
      'Host': 'www.ncat3.app',
      'Connection': 'keep-alive',
    });
    expect(split.userAgent, 'Site/1');
    expect(split.referer, 'https://site.example/');
    expect(split.fields.containsKey('User-Agent'), isFalse);
    expect(split.fields.containsKey('Referer'), isFalse);
    expect(split.fields['Cookie'], 'a=b');
    expect(split.fields['Origin'], 'https://site.example');
    expect(split.fields['Range'], 'bytes=0-1');
    expect(split.fields['Host'], 'www.ncat3.app');
    expect(split.fields['Connection'], 'keep-alive');
    final kept = kotvNormalizePlayHeaders(
      const {
        'Host': 'www.ncat3.app',
        'Connection': 'keep-alive',
        'Cookie': 'a=1',
        'Referer': 'https://www.ncat3.app',
      },
      url: 'https://vip.example/a.m3u8',
      addDefaultUa: false,
    );
    expect(kept['Host'], 'www.ncat3.app');
    expect(kept['Connection'], 'keep-alive');
    expect(kept['Cookie'], 'a=1');
    expect(kept['Referer'], 'https://www.ncat3.app');
    final empty = kotvSplitMpvHeaders(const {});
    expect(empty.userAgent, isEmpty);
    expect(empty.referer, isEmpty);
    expect(empty.fields, isEmpty);
  });

  test('empty site header uses okhttp ua, settings ua wins', () {
    kotvApplyPlayUaSetting('');
    expect(kotvDefaultPlayUA, 'okhttp/5.5.0');
    expect(
      kotvChromePlayUA,
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/155.0.0.0 Safari/537.36',
    );
    expect(
      kotvNormalizePlayHeaders(const {}, url: 'https://cdn.example/a.m3u8')['User-Agent'],
      kotvIsDesktop() ? isNull : 'okhttp/5.5.0',
    );
    kotvSetPlatformPlayUa('com.bobo.kotv/1.2.3 (Linux;Android 16) ExoPlayerLib/1.11.0');
    expect(
      kotvNormalizePlayHeaders(const {}, url: 'https://cdn.example/a.m3u8')['User-Agent'],
      'com.bobo.kotv/1.2.3 (Linux;Android 16) ExoPlayerLib/1.11.0',
    );
    kotvSetPlatformPlayUa('');
    kotvApplyPlayUaSetting('CustomPlayer/1');
    expect(
      kotvNormalizePlayHeaders(const {}, url: 'https://cdn.example/a.m3u8')['User-Agent'],
      'CustomPlayer/1',
    );
    expect(
      kotvNormalizePlayHeaders(
        const {'User-Agent': 'from-site'},
        url: 'https://cdn.example/a.m3u8',
      )['User-Agent'],
      'from-site',
    );
    kotvApplyPlayUaSetting('');
  });
}
