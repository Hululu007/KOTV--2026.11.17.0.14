import 'package:flutter_test/flutter_test.dart';
import 'package:kotv/player/kotv_playback.dart';
import 'package:kotv/player/mpv_hls_timeline.dart';

void main() {
  test('m3u8 path and query use lavf hls', () {
    expect(kotvIsHls('https://cdn/a.f300.m3u8', null), isTrue);
    expect(kotvIsHls('https://cdn/a.f300.m3u8/', null), isTrue);
    // 任意 .php 不再预强制；伪装节目单靠打开失败再开。
    expect(kotvIsHls('https://cdn/play.php', null), isFalse);
    expect(kotvIsHls('https://cdn/play.php/', null), isFalse);
    expect(kotvIsHls('https://cdn/play.php?x=m3u8', null), isTrue);
    expect(kotvIsHls('https://cdn/m3u8', null), isTrue);
    expect(kotvIsHls('https://cdn/m3u8/', null), isTrue);
    expect(kotvIsHls('https://cdn/v?file=m3u8', null), isTrue);
    // 查询参数值里的 .m3u8 不算路径扩展名（回归样例，非站点特例）。
    expect(kotvIsHls('https://cdn/fengshows?id=2.m3u8', null), isFalse);
    expect(kotvIsHls('http://111.1.1.1:9090/nby/m3u8/getM3u8?name=x&url=NBY-1.m3u8', null), isTrue);
    expect(kotvIsHls('https://cdn/api/getM3u8?id=1', null), isTrue);
    expect(kotvIsHls('https://cdn/a.mp4', null), isFalse);
    expect(kotvIsHls('https://cdn/a.m3u', null), isFalse);
    expect(kotvIsHls('rtsp://cdn/a.m3u8', null), isFalse);
    expect(kotvIsHls('https://cdn/live.m3u8?type=mpd', null), isFalse);
    expect(kotvIsHls('data:application/dash+xml,abc', null), isFalse);
    expect(kotvIsHls('data:application/vnd.apple.mpegurl,abc', null), isTrue);
  });

  test('only application/x-mpegURL counts as hls mime', () {
    expect(kotvIsHls('https://cdn/a.mp4', 'application/x-mpegURL'), isTrue);
    expect(kotvIsHls('https://cdn/a.m3u8', 'video/mp4'), isTrue);
    expect(kotvPlayMime('https://cdn/a.m3u8', 'video/mp4'), 'video/mp4');
    expect(kotvPlayMime('https://cdn/a.m3u8', null), 'application/x-mpegURL');
    expect(kotvPlayMime('https://cdn/a.mp4', 'application/x-mpegURL'), 'application/x-mpegURL');
    expect(kotvIsHls('https://cdn/a.mp4', 'application/vnd.apple.mpegurl'), isFalse);
    expect(kotvIsHls('https://cdn/a.mp4', 'hls'), isFalse);
    expect(kotvIsHls('https://cdn/a.mp4', 'm3u8'), isFalse);
    expect(kotvIsHls('https://cdn/a.mp4', 'video/mp4'), isFalse);
    expect(kotvPlayMime('https://cdn/a.mp4', 'video/mp4'), 'video/mp4');
    expect(kotvPlayMime('https://cdn/a.mp4', null), isNull);
  });

  test('open failure text', () {
    expect(kotvMpvHlsOpenFailed('lavf', 'avformat_open_input() failed'), isTrue);
    expect(
      kotvMpvHlsOpenFailed(
        'ffmpeg',
        'Not detecting m3u8/hls with non standard extension and non standard mime type',
      ),
      isTrue,
    );
    expect(kotvMpvHlsOpenFailed('cplayer', 'Failed to recognize file format.'), isFalse);
    expect(kotvMpvNetworkUrl('https://cdn/a.mp4'), isTrue);
    expect(kotvMpvNetworkUrl('file:///tmp/a.mp4'), isFalse);
    expect(kotvMpvHlsOpenFailed('lavf', 'could not open codec'), isFalse);
    expect(kotvMpvHlsOpenFailed('vd', 'could not open codec'), isFalse);
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
}
