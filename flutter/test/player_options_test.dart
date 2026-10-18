import 'package:flutter_test/flutter_test.dart';
import 'package:kotv/player/kotv_platform.dart';

void main() {
  test('external players stay hidden until installed', () {
    const opts = [
      ('内置 MPV', 'innie#mpv'),
      ('外部 MPV', 'outie#mpv'),
      ('外部 VLC', 'outie#vlc'),
      ('外部播放器', 'outie#view'),
    ];
    final got = kotvVisiblePlayerOptions(opts, const {'mpv': true});
    expect(got.map((e) => e.$2).toList(), ['innie#mpv', 'outie#mpv']);
  });

  test('built-in players stay when availability is empty', () {
    const opts = [
      ('内置 ExoPlayer', 'innie#exo'),
      ('外部播放器', 'outie#view'),
    ];
    final got = kotvVisiblePlayerOptions(opts, const {});
    expect(got.map((e) => e.$2).toList(), ['innie#exo']);
  });
}
