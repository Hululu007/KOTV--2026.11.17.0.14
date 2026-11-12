import 'package:flutter_test/flutter_test.dart';
import 'package:kotv/player/play_format.dart';

void main() {
  test('vod sum EXTINF ignores DISCONTINUITY', () {
    const body = '''
#EXTM3U
#EXTINF:10.0,
a.ts
#EXT-X-DISCONTINUITY
#EXTINF:5.5,
b.ts
#EXT-X-DISCONTINUITY
#EXTINF:4.5,
c.ts
#EXT-X-ENDLIST
''';
    expect(kotvHlsVodDurationMs(body), 20000);
  });

  test('live without ENDLIST returns null', () {
    const body = '''
#EXTM3U
#EXTINF:6.0,
a.ts
#EXTINF:6.0,
b.ts
''';
    expect(kotvHlsVodDurationMs(body), isNull);
  });

  test('master playlist returns null and first variant uri', () {
    const body = '''
#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000
media.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=400000
low.m3u8
''';
    expect(kotvHlsVodDurationMs(body), isNull);
    expect(kotvHlsFirstVariantUri(body), 'media.m3u8');
  });

  test('prefer playlist only when engine timeline already near full', () {
    // 引擎仍是单片长度：勿用节目单撑满进度条。
    expect(
      kotvPreferPlaylistDuration(
        const Duration(seconds: 10),
        const Duration(minutes: 40),
      ),
      const Duration(seconds: 10),
    );
    // 引擎已接近节目单：可用加总修正偏短 demux。
    expect(
      kotvPreferPlaylistDuration(
        const Duration(seconds: 700),
        const Duration(seconds: 800),
      ),
      const Duration(seconds: 800),
    );
    expect(
      kotvPreferPlaylistDuration(
        const Duration(seconds: 30),
        const Duration(seconds: 20),
      ),
      const Duration(seconds: 30),
    );
    expect(
      kotvPreferPlaylistDuration(const Duration(seconds: 10), null),
      const Duration(seconds: 10),
    );
    expect(
      kotvPreferPlaylistDuration(Duration.zero, const Duration(minutes: 40)),
      Duration.zero,
    );
  });

  test('live media playlist without ENDLIST', () {
    const live = '''
#EXTM3U
#EXTINF:6.0,
a.ts
#EXTINF:6.0,
b.ts
''';
    expect(kotvHlsBodyIsLiveMediaPlaylist(live), isTrue);
    const vod = '''
#EXTM3U
#EXTINF:6.0,
a.ts
#EXT-X-ENDLIST
''';
    expect(kotvHlsBodyIsLiveMediaPlaylist(vod), isFalse);
    const master = '''
#EXTM3U
#EXT-X-STREAM-INF:BANDWIDTH=800000
media.m3u8
''';
    expect(kotvHlsBodyIsLiveMediaPlaylist(master), isFalse);
  });

  test('fetch skips live and non-hls', () async {
    expect(
      await kotvFetchHlsVodDurationMs(
        url: 'https://example.com/a.m3u8',
        headers: const {},
        live: true,
      ),
      isNull,
    );
    expect(
      await kotvFetchHlsVodDurationMs(
        url: 'https://example.com/a.mp4',
        headers: const {},
        live: false,
      ),
      isNull,
    );
  });

  test('refresh helper skips live and stale current', () async {
    expect(
      await kotvRefreshHlsPlaylistDuration(
        url: 'https://example.com/a.m3u8',
        headers: const {},
        live: true,
        format: null,
        stillCurrent: () => true,
      ),
      isNull,
    );
    expect(
      await kotvRefreshHlsPlaylistDuration(
        url: 'https://example.com/a.mp4',
        headers: const {},
        live: false,
        format: null,
        stillCurrent: () => false,
      ),
      isNull,
    );
  });
}
