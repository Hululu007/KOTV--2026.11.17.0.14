import 'package:flutter_test/flutter_test.dart';
import 'package:kotv/player/kotv_platform.dart';
import 'package:kotv/player/play_headers.dart';

void main() {
  test('preferDirect opens the site url', () {
    final got = kotvResolvePlayOpenTarget(
      playUrl: 'http://127.0.0.1:9978/proxy/play?id=1',
      directUrl: 'https://cdn.example/live.m3u8',
      headers: const {'Cookie': 'a=b'},
      magnet: false,
      sendHeaders: true,
      preferDirect: true,
    );
    expect(got.url, 'https://cdn.example/live.m3u8');
    expect(got.headers?['Cookie'], 'a=b');
  });

  test('without preferDirect keeps the engine url', () {
    final got = kotvResolvePlayOpenTarget(
      playUrl: 'http://127.0.0.1:9978/proxy/play?id=1',
      directUrl: 'https://cdn.example/live.m3u8',
      headers: const {'Cookie': 'a=b'},
      magnet: false,
      sendHeaders: true,
      preferDirect: false,
    );
    expect(got.url, contains('/proxy/play'));
    expect(got.headers, isNull);
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
      preferDirect: true,
    );
    expect(got.url, contains('/proxy/play'));
    expect(got.headers, isNull);
  });

  test('prefer and switch-to-proxy gates', () {
    expect(
      kotvShouldPreferPlayDirect(playerVal: 'innie#exo', adFilterOn: true),
      isTrue,
    );
    expect(
      kotvShouldPreferPlayDirect(playerVal: 'innie#mpv', adFilterOn: true),
      isFalse,
    );
    expect(
      kotvShouldPreferPlayDirect(playerVal: 'innie#mpv', adFilterOn: false),
      isTrue,
    );
    expect(
      kotvShouldSwitchToProxyAfterOpenFailure(
        playerVal: 'innie#mpv',
        openUrl: 'https://cdn.example/a.m3u8',
        playUrl: 'http://127.0.0.1:9978/proxy/play?id=1',
        alreadySwitched: false,
      ),
      isTrue,
    );
    expect(
      kotvShouldSwitchToProxyAfterOpenFailure(
        playerVal: 'innie#exo',
        openUrl: 'https://cdn.example/a.m3u8',
        playUrl: 'http://127.0.0.1:9978/proxy/play?id=1',
        alreadySwitched: false,
      ),
      isFalse,
    );
    expect(
      kotvShouldSwitchToProxyAfterOpenFailure(
        playerVal: 'innie#html',
        openUrl: 'https://cdn.example/a.m3u8',
        playUrl: 'https://cdn.example/a.m3u8',
        alreadySwitched: false,
      ),
      isFalse,
    );
  });

  test('preferred open uses direct only when preferDirect', () {
    final direct = kotvResolvePreferredPlayOpen(
      playerVal: 'innie#mpv',
      playUrl: 'http://127.0.0.1:9978/proxy/play?id=1',
      directUrl: 'https://cdn.example/a.m3u8',
      rawHeaders: const {'Cookie': 'a=b'},
      magnet: false,
      format: 'application/x-mpegURL',
      adFilterOn: false,
    );
    expect(direct.url, 'https://cdn.example/a.m3u8');
    expect(direct.headers?['Cookie'], 'a=b');

    final proxied = kotvResolvePreferredPlayOpen(
      playerVal: 'innie#mpv',
      playUrl: 'http://127.0.0.1:9978/proxy/play?id=1',
      directUrl: 'https://cdn.example/a.m3u8',
      rawHeaders: const {'Cookie': 'a=b'},
      magnet: false,
      format: 'application/x-mpegURL',
      adFilterOn: true,
    );
    expect(proxied.url, contains('/proxy/play'));
    expect(proxied.headers, isNull);

    final forced = kotvResolvePreferredPlayOpen(
      playerVal: 'innie#mpv',
      playUrl: 'http://127.0.0.1:9978/proxy/play?id=1',
      directUrl: 'https://cdn.example/a.m3u8',
      rawHeaders: const {'Cookie': 'a=b'},
      magnet: false,
      adFilterOn: false,
      forceProxy: true,
    );
    expect(forced.url, contains('/proxy/play'));
  });

  test('ephemeral play urls match engine proxy paths', () {
    expect(kotvIsEphemeralPlayUrl('http://127.0.0.1:9978/proxy/play?id=1'), isTrue);
    expect(kotvIsEphemeralPlayUrl('http://127.0.0.1:9978/proxy/bt/x'), isTrue);
    expect(kotvIsEphemeralPlayUrl('http://127.0.0.1:9978/proxy/hls/x'), isTrue);
    expect(kotvIsEphemeralPlayUrl('http://127.0.0.1:9978/proxy/rolling_m3u8?id=1'), isTrue);
    expect(kotvIsEphemeralPlayUrl('https://cdn.example/a.m3u8'), isFalse);
    expect(kotvIsLocalProxyUrl('http://127.0.0.1:9978/proxy/cached_m3u8?id=1'), isTrue);
    expect(kotvIsLocalProxyUrl('http://192.168.1.8:9978/proxy?do=quark'), isTrue);
    expect(kotvIsLocalProxyUrl('https://cdn.example/a.m3u8'), isFalse);
  });

  test('exo mpv always send headers; web hls.js only on playlists', () {
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
    expect(split.fields.containsKey('Range'), isFalse);
    expect(split.fields['Cookie'], 'a=b');
    final empty = kotvSplitMpvHeaders(const {});
    expect(empty.userAgent, isEmpty);
    expect(empty.referer, isEmpty);
    expect(empty.fields, isEmpty);
  });

  test('normalize strips Range leftover', () {
    final got = kotvNormalizePlayHeaders(
      const {
        'Range': 'bytes=0-1',
        'Cookie': 'a=b',
      },
      url: 'https://cdn.example/a.mp4',
      addDefaultUa: false,
    );
    expect(got.containsKey('Range'), isFalse);
    expect(got['Cookie'], 'a=b');
  });

  test('empty site header uses okhttp ua, settings ua wins', () {
    kotvApplyPlayUaSetting('');
    expect(kotvDefaultPlayUA, 'okhttp/5.5.0');
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
    kotvApplyPlayUaSetting('');
  });
}
