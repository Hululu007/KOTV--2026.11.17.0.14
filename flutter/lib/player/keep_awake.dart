import 'package:wakelock_plus/wakelock_plus.dart';

/// 播放中保持屏幕常亮（防休眠 / 屏保 / 自动锁屏）。
///
/// 手机与桌面均走 [WakelockPlus]：
/// - Android/iOS：窗口 FLAG_KEEP_SCREEN_ON / idle timer
/// - Windows：SetThreadExecutionState
/// - macOS：IOPMAssertion
/// - Linux：DBus screensaver inhibit
///
/// 用持有者集合做引用计数，详情/直播/多引擎切换不会互相抢关。
class KotvKeepAwake {
  KotvKeepAwake._();

  static final Set<Object> _holders = <Object>{};
  static bool? _applied;

  /// [holder] 通常是 [KotvPlayback] 实例；[hold]=true 表示该实例正在播/缓冲。
  static void setHolding(Object holder, bool hold) {
    final before = _holders.isNotEmpty;
    if (hold) {
      _holders.add(holder);
    } else {
      _holders.remove(holder);
    }
    final after = _holders.isNotEmpty;
    if (before == after && _applied == after) return;
    _applied = after;
    if (after) {
      WakelockPlus.enable().catchError((_) {});
    } else {
      WakelockPlus.disable().catchError((_) {});
    }
  }
}
