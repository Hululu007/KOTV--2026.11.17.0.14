import 'package:flutter_test/flutter_test.dart';
import 'package:kotv/player/mpv_opts.dart';

void main() {
  test('appendLavfHlsLoadOpts only for HLS', () {
    final none = <String>[];
    kotvAppendLavfHlsLoadOpts('https://cdn.example/a.mp4', null, none);
    expect(none, isEmpty);

    final uriHls = <String>[];
    kotvAppendLavfHlsLoadOpts('https://cdn.example/a.m3u8', null, uriHls);
    expect(uriHls, [
      'demuxer-lavf-format=hls',
      'demuxer-lavf-o-add=$kotvHlsLavfOAddForUri',
    ]);

    final mimeHls = <String>[];
    kotvAppendLavfHlsLoadOpts(
      'https://proxy.example/play',
      'application/x-mpegURL',
      mimeHls,
    );
    expect(mimeHls, [
      'demuxer-lavf-format=hls',
      'demuxer-lavf-o-add=$kotvHlsLavfOAddForMime',
    ]);
  });

  test('encodeMpvLoadfileOptions wraps utf8 byte lengths', () {
    expect(kotvEncodeMpvLoadfileOptions(const []), '');
    expect(
      kotvEncodeMpvLoadfileOptions(const ['start=12.5']),
      '%5%start=%4%12.5',
    );
    expect(
      kotvEncodeMpvLoadfileOptions(const [
        'demuxer-lavf-format=hls',
        'demuxer-lavf-o-add=http_persistent=0',
      ]),
      '%19%demuxer-lavf-format=%3%hls,'
      '%18%demuxer-lavf-o-add=%17%http_persistent=0',
    );
  });
}
