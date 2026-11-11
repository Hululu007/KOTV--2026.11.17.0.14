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

  test('prefer playlist over shorter demux', () {
    expect(
      kotvPreferPlaylistDuration(
        const Duration(seconds: 10),
        const Duration(seconds: 20),
      ),
      const Duration(seconds: 20),
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
}
