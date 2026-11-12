import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

import 'kotv_platform.dart';

/// 各播放器共用的前向缓冲字节预算。
///
/// 前向到达上限就停止继续读。已经播过、超出回看保留的数据会丢掉，空出来的额度继续往前补。
/// mpv：`demuxer-max-bytes` 是前向上限，`demuxer-max-back-bytes` 是保留的已播数据。
/// Exo：`targetBufferBytes` 或最长缓冲时长到了就停读；回看缓冲时长为 0，播放位置之前的样本会丢掉。
/// FVP：队列时长到了就不再推包；`drop` 为 false 时等解码把队列消化到上限以下。
/// 磁盘缓存只在点播且设置开启时另走。不设 mpv `cache-secs` / `demuxer-readahead-secs`。
/// Web 和外部播放器不走这份预算。
///
/// 预算按当前可用内存的比例：约 15% 可用内存，且不超过总内存的 5%。
/// 不设固定字节上下限。读不到总内存时，桌面按 8GiB、其它按 3GiB 估算后再取 5%。
class KotvBufferBudget {
  KotvBufferBudget._();

  static const _android = MethodChannel('kotv_android');
  static const _host = MethodChannel('kotv_host');
  static int? _cached;

  /// mpv 缓冲：仅字节预算；`demuxer-max-bytes` 是上限不是起播门槛。点播和直播共用。
  ///
  /// 不含 `cache-on-disk`：由 [KotvMpvOpts.diskCache] / 原生 `diskCache` 单独控制，
  /// 避免预算表把磁盘缓存开关盖掉。直播的 `cache-pause=no` 由调用方另写。
  static Map<String, String> mpvCacheProps(int budgetBytes) {
    final forward = mpvMiB(budgetBytes);
    final back = mpvMiB(budgetBytes ~/ 8);
    return {
      'cache': 'yes',
      'demuxer-max-bytes': forward,
      'demuxer-max-back-bytes': back,
      'cache-pause-initial': 'no',
      'framedrop': 'vo',
    };
  }

  /// 同步读取（未 [warm] 时用平台启发式）。
  static int bytes() => _cached ?? _fallback();

  /// 读取/刷新预算。各平台读真实 total/avail；[force] 时按当前可用内存重算。
  static Future<int> warm({bool force = false}) async {
    if (!force && _cached != null) return _cached!;
    try {
      Map<dynamic, dynamic>? raw;
      if (kotvIsAndroid()) {
        final v = await _android.invokeMethod<dynamic>('getMemoryInfo');
        if (v is Map) raw = v;
      } else if (kotvIsDesktop() || kotvIsIOS()) {
        final v = await _host.invokeMethod<dynamic>('getMemoryInfo');
        if (v is Map) raw = v;
      }
      if (raw != null) {
        final total = (raw['totalBytes'] as num?)?.toInt() ?? 0;
        final avail = (raw['availBytes'] as num?)?.toInt() ?? 0;
        if (total > 0) {
          _cached = fromDevice(
            totalBytes: total,
            availBytes: avail,
            desktop: kotvIsDesktop(),
          );
          return _cached!;
        }
      }
    } catch (_) {}
    _cached = _fallback();
    return _cached!;
  }

  /// 有可用内存时取 min(可用 × 15%, 总内存 × 5%)；没有可用内存时用总内存的 5%。
  ///
  /// 总内存也没有时，按桌面 8GiB、其它 3GiB 估算后再取 5%。
  static int fromDevice({
    required int totalBytes,
    required int availBytes,
    required bool desktop,
  }) {
    final total = totalBytes > 0
        ? totalBytes
        : (desktop ? 8 * 1024 * 1024 * 1024 : 3 * 1024 * 1024 * 1024);
    var budget = (total * 0.05).round();
    if (availBytes > 0) {
      final byAvail = (availBytes * 0.15).round();
      if (byAvail < budget) budget = byAvail;
    }
    if (budget < 1) return 1;
    return budget;
  }

  static int _fallback() => fromDevice(
        totalBytes: 0,
        availBytes: 0,
        desktop: kotvIsDesktop(),
      );

  /// mpv `demuxer-max-bytes` 风格：`96MiB`。不足 1MiB 写成 `1MiB`，避免 0 被当成不限制。
  static String mpvMiB(int n) {
    var mib = (n / (1024 * 1024)).round();
    if (mib < 1) mib = 1;
    return '${mib}MiB';
  }

  /// mdk [setBufferRange] 只有毫秒。把内存预算按这条流的码率换成能放下的媒体时长。
  ///
  /// [bitsPerSec] 来自容器或音视频轨。为 0 时返回 0，调用方用临时上限并继续等码率。
  /// 返回值至少 1：0 在 mdk 里表示不限制时长（且与 drop 组合时可能只留关键帧）。
  static int fvpMaxBufferMs(int budgetBytes, {required int bitsPerSec}) {
    if (budgetBytes <= 0 || bitsPerSec <= 0) return 0;
    final bytesPerSec = bitsPerSec / 8.0;
    var ms = (budgetBytes / bytesPerSec * 1000.0).round();
    if (ms < 1) ms = 1;
    if (ms > 2 * 3600 * 1000) return 2 * 3600 * 1000;
    return ms;
  }

  @visibleForTesting
  static void debugReset() => _cached = null;
}
