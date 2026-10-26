import 'unsupported_playback_stub.dart';

/// 非 Web：ArtPlayer 不可用。
class ArtPlayback extends UnsupportedPlayback {
  ArtPlayback() : super(engineLabel: 'ArtPlayer', reason: 'ArtPlayer 仅支持 Web');
}
