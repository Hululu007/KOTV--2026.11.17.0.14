import 'package:flutter_test/flutter_test.dart';
import 'package:kotv/player/kotv_platform.dart';

void main() {
  test('external view stays hidden until available', () {
    const opts = [
      ('内置 MPV', 'innie#mpv'),
      ('外部播放器', 'outie#view'),
    ];
    final hidden = kotvVisiblePlayerOptions(opts, const {});
    expect(hidden.map((e) => e.$2).toList(), ['innie#mpv']);
    final shown = kotvVisiblePlayerOptions(opts, const {'view': true});
    expect(shown.map((e) => e.$2).toList(), ['innie#mpv', 'outie#view']);
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
