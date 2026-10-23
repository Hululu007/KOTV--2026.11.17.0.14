import 'package:flutter_test/flutter_test.dart';
import 'package:kotv/player/mpv_hls_timeline.dart';

void main() {
  test('m3u8 path and query use lavf hls', () {
    expect(kotvMpvHlsLavf('https://cdn/a.f300.m3u8', null), isTrue);
    expect(kotvMpvHlsLavf('https://cdn/a.f300.m3u8/', null), isTrue);
    expect(kotvMpvHlsLavf('https://cdn/play.php', null), isTrue);
    expect(kotvMpvHlsLavf('https://cdn/play.php/', null), isTrue);
    expect(kotvMpvHlsLavf('https://cdn/m3u8', null), isTrue);
    expect(kotvMpvHlsLavf('https://cdn/m3u8/', null), isTrue);
    expect(kotvMpvHlsLavf('https://cdn/v?file=m3u8', null), isTrue);
    expect(kotvMpvHlsLavf('https://cdn/fengshows?id=2.m3u8', null), isFalse);
    expect(kotvMpvHlsLavf('https://cdn/a.mp4', null), isFalse);
    expect(kotvMpvHlsLavf('https://cdn/a.m3u', null), isFalse);
    expect(kotvMpvHlsLavf('rtsp://cdn/a.m3u8', null), isFalse);
    expect(kotvMpvHlsLavf('https://cdn/live.m3u8?type=mpd', null), isFalse);
    expect(kotvMpvHlsLavf('data:application/dash+xml,abc', null), isFalse);
    expect(kotvMpvHlsLavf('data:application/vnd.apple.mpegurl,abc', null), isTrue);
  });

  test('only application/x-mpegURL counts as hls mime', () {
    expect(kotvMpvHlsLavf('https://cdn/a.mp4', 'application/x-mpegURL'), isTrue);
    expect(kotvMpvHlsLavf('https://cdn/a.m3u8', 'video/mp4'), isTrue);
    expect(kotvPlayMime('https://cdn/a.m3u8', 'video/mp4'), 'application/x-mpegURL');
    expect(kotvPlayMime('https://cdn/a.mp4', 'application/x-mpegURL'), 'application/x-mpegURL');
    expect(kotvMpvHlsLavf('https://cdn/a.mp4', 'application/vnd.apple.mpegurl'), isFalse);
    expect(kotvMpvHlsLavf('https://cdn/a.mp4', 'hls'), isFalse);
    expect(kotvMpvHlsLavf('https://cdn/a.mp4', 'm3u8'), isFalse);
    expect(kotvMpvHlsLavf('https://cdn/a.mp4', 'video/mp4'), isFalse);
    expect(kotvPlayMime('https://cdn/a.mp4', 'video/mp4'), 'video/mp4');
    expect(kotvPlayMime('https://cdn/a.mp4', null), isNull);
  });

  test('open failure text', () {
    expect(kotvMpvHlsOpenFailed('lavf', 'avformat_open_input() failed'), isTrue);
    expect(kotvMpvHlsOpenFailed('cplayer', 'Failed to recognize file format.'), isTrue);
    expect(kotvMpvHlsOpenFailed('lavf', 'could not open codec'), isFalse);
    expect(kotvMpvHlsOpenFailed('vd', 'could not open codec'), isFalse);
  });
}
