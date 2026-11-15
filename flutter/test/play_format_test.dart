import 'package:flutter_test/flutter_test.dart';
import 'package:kotv/player/kotv_playback.dart';
import 'package:kotv/player/play_format.dart';

void main() {
  test('m3u8 path and query are HLS', () {
    expect(kotvIsHls('https://cdn/a.f300.m3u8', null), isTrue);
    expect(kotvIsHls('https://cdn/a.f300.m3u8/', null), isTrue);
    expect(kotvIsHls('https://cdn/play.php', null), isFalse);
    expect(kotvIsHls('https://cdn/play.php/', null), isFalse);
    expect(kotvIsHls('https://cdn/play.php?x=m3u8', null), isTrue);
    expect(kotvIsHls('https://cdn/play.php?X=M3U8', null), isTrue);
    expect(kotvIsHls('https://cdn/m3u8', null), isTrue);
    expect(kotvIsHls('https://cdn/m3u8/', null), isTrue);
    expect(kotvIsHls('https://cdn/v?file=m3u8', null), isTrue);
    expect(kotvIsHls('https://cdn/fengshows?id=2.m3u8', null), isFalse);
    expect(kotvIsHls('http://111.1.1.1:9090/nby/m3u8/getM3u8?name=x&url=NBY-1.m3u8', null), isTrue);
    expect(kotvIsHls('https://cdn/api/getM3u8?id=1', null), isTrue);
    expect(kotvIsHls('https://cdn/a.mp4', null), isFalse);
    expect(kotvIsHls('https://cdn/a.m3u', null), isFalse);
    expect(kotvIsHls('rtsp://cdn/a.m3u8', null), isFalse);
    expect(kotvIsHls('https://cdn/live.m3u8?type=mpd', null), isFalse);
    expect(kotvIsHls('data:application/dash+xml,abc', null), isFalse);
    expect(kotvIsHls('data:video/mp4,abc', null), isFalse);
    expect(kotvIsHls('data:application/vnd.apple.mpegurl,abc', null), isTrue);
  });

  test('play mime matches hls decision for all players', () {
    expect(kotvIsHls('https://cdn/a.mp4', 'application/x-mpegURL'), isTrue);
    expect(kotvIsHls('https://cdn/a.m3u8', 'video/mp4'), isTrue);
    expect(kotvPlayMime('https://cdn/a.m3u8', 'video/mp4'), 'application/x-mpegURL');
    expect(kotvPlayMime('https://cdn/a.m3u8', null), 'application/x-mpegURL');
    expect(kotvPlayMime('https://cdn/a.mp4', 'application/x-mpegURL'), 'application/x-mpegURL');
    expect(kotvIsHls('https://cdn/a.mp4', 'application/vnd.apple.mpegurl'), isTrue);
    expect(kotvIsHls('https://cdn/a.mp4', 'hls'), isTrue);
    expect(kotvIsHls('https://cdn/a.mp4', 'm3u8'), isTrue);
    expect(kotvPlayMime('https://cdn/a.mp4', 'hls'), 'application/x-mpegURL');
    expect(kotvIsHls('https://cdn/a.mp4', 'video/mp4'), isFalse);
    expect(kotvPlayMime('https://cdn/a.mp4', 'video/mp4'), 'video/mp4');
    expect(kotvPlayMime('https://cdn/a.mp4', null), isNull);
    expect(kotvPlayMime('https://cdn/play.php', null), isNull);
  });

  test('mpv hls retry on io when not already hls', () {
    expect(
      kotvShouldRetryMpvHlsAfterError(
        url: 'https://cdn/play.php?id=1',
        format: null,
        alreadyRetried: false,
        errorText: 'tcp: Connection refused',
      ),
      isTrue,
    );
    expect(
      kotvShouldRetryMpvHlsAfterError(
        url: 'https://cdn/play.php?id=1',
        format: 'video/mp4',
        alreadyRetried: false,
        errorText: 'tcp: Connection refused',
      ),
      isTrue,
    );
    expect(
      kotvShouldRetryMpvHlsAfterError(
        url: 'https://cdn/a.m3u8',
        format: null,
        alreadyRetried: false,
        errorText: 'tcp: Connection refused',
      ),
      isFalse,
    );
    expect(
      kotvShouldRetryMpvHlsAfterError(
        url: 'https://cdn/play.php?id=1',
        format: 'application/x-mpegURL',
        alreadyRetried: false,
        errorText: 'failed to open',
      ),
      isFalse,
    );
    expect(
      kotvShouldRetryMpvHlsAfterError(
        url: 'https://cdn/play.php?id=1',
        format: null,
        alreadyRetried: true,
        errorText: 'tcp: Connection refused',
      ),
      isFalse,
    );
  });

  test('image media codecs are not real video tracks', () {
    expect(kotvIsImageMediaCodec('png'), isTrue);
    expect(kotvIsImageMediaCodec('mjpeg'), isTrue);
    expect(kotvIsImageMediaCodec('h264'), isFalse);
    expect(kotvIsImageMediaCodec(null), isFalse);
    expect(kotvIsRealVideoTrack(id: '1', codec: 'png'), isFalse);
    expect(kotvIsRealVideoTrack(id: '1', codec: 'h264'), isTrue);
    expect(kotvIsRealVideoTrack(id: 'auto', codec: 'h264'), isFalse);
    expect(kotvIsRealVideoTrack(id: '1', codec: 'h264', image: true), isFalse);
  });

  test('live guard only trusts marked or engine live', () {
    expect(kotvIsLiveForGuard(markedLive: false), isFalse);
    expect(kotvIsLiveForGuard(markedLive: true), isTrue);
    expect(kotvIsLiveForGuard(markedLive: false, engineLive: true), isTrue);
    expect(kotvIsLiveForGuard(markedLive: false, engineLive: false), isFalse);
  });

  test('growing duration marks live after two steps', () {
    var g = 0;
    final a = kotvNoteGrowingDuration(
      previous: const Duration(seconds: 8),
      next: const Duration(seconds: 10),
      growthCount: g,
    );
    expect(a.markLive, isFalse);
    g = a.growthCount;
    final b = kotvNoteGrowingDuration(
      previous: const Duration(seconds: 10),
      next: const Duration(seconds: 12),
      growthCount: g,
    );
    expect(b.markLive, isTrue);
    expect(b.growthCount, 2);
  });

  test('tiny duration jitter does not count as growth', () {
    final r = kotvNoteGrowingDuration(
      previous: const Duration(milliseconds: 10000),
      next: const Duration(milliseconds: 10200),
      growthCount: 1,
    );
    expect(r.markLive, isFalse);
    expect(r.growthCount, 1);
  });
}
