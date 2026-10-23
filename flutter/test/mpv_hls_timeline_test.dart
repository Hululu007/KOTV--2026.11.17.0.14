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

  test('redirected url drops the previous format', () {
    expect(
      kotvMpvForceLavf(
        'https://gw/live',
        'https://cdn/a.flv',
        'application/x-mpegURL',
      ),
      isFalse,
    );
    expect(
      kotvMpvForceLavf(
        'https://gw/live',
        'https://cdn/a.m3u8',
        'video/mp4',
      ),
      isTrue,
    );
    expect(
      kotvMpvForceLavf('https://cdn/a.mp4', 'https://cdn/a.mp4', 'hls'),
      isTrue,
    );
  });

  test('format hls uses lavf even without the suffix', () {
    expect(kotvMpvHlsLavf('https://cdn/a.mp4', 'application/x-mpegURL'), isTrue);
    expect(kotvMpvHlsLavf('https://cdn/a.mp4', 'application/vnd.apple.mpegurl'), isTrue);
    expect(kotvMpvHlsLavf('https://cdn/a.mp4', 'hls'), isTrue);
    expect(kotvMpvHlsLavf('https://cdn/a.mp4', 'm3u8'), isTrue);
    expect(kotvMpvHlsLavf('https://cdn/a.mp4', 'video/mp4'), isFalse);
  });

  test('open failure text', () {
    expect(kotvMpvHlsOpenFailed('lavf', 'avformat_open_input() failed'), isTrue);
    expect(kotvMpvHlsOpenFailed('cplayer', 'Failed to recognize file format.'), isTrue);
    expect(kotvMpvHlsOpenFailed('lavf', 'could not open codec'), isFalse);
    expect(kotvMpvHlsOpenFailed('vd', 'could not open codec'), isFalse);
  });
}
