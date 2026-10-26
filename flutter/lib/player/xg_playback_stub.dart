import 'unsupported_playback_stub.dart';

/// 非 Web：xgplayer 不可用。
class XgPlayback extends UnsupportedPlayback {
  XgPlayback() : super(engineLabel: 'xgplayer', reason: 'xgplayer 仅支持 Web');
}
