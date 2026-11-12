import 'unsupported_playback_stub.dart';

/// Web：FVP / libmdk 不可用（依赖 dart:ffi）。
class FvpPlayback extends UnsupportedPlayback {
  FvpPlayback() : super(engineLabel: '内置 FVP', reason: 'FVP 不支持 Web');

  /// 与 IO 侧同名，避免条件导入缺方法。
  Future<void> pauseForLiveZap() async {}

  /// Web 无 FVP：与 IO 侧同名，避免 dart2js 因条件导入缺方法失败。
  void applyPlayerOptions(Map<String, dynamic> settings) {}
}
