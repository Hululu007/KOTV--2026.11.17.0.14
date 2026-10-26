import 'unsupported_playback_stub.dart';

/// 非 Web：ZWPlayer 不可用。
class ZwPlayback extends UnsupportedPlayback {
  ZwPlayback() : super(engineLabel: 'ZWPlayer', reason: 'ZWPlayer 仅支持 Web');
}
