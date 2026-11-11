/// 播放容器判定（各播放器共用）。
///
/// 正文探测在引擎 `planPlayback`；客户端以回包 format 为主，URL 仅强信号快路径。
/// Dart / Kotlin 不能调 Go，URL/format 规则在此与 `KotvMpvHls` 各留一份副本。

/// URL 强信号快路径（规范见 Go `model.URLIsHls`）。
bool kotvUrlIsHls(String url) {
  final raw = url.trim();
  if (raw.isEmpty) return false;
  final uri = Uri.tryParse(raw);
  if (uri == null) return raw.toLowerCase().contains('=m3u8');
  final scheme = uri.scheme.toLowerCase();
  if (scheme == 'rtsp' || scheme == 'rtspt') return false;
  if (scheme == 'data') {
    final body = raw.length > 5 ? raw.substring(5) : '';
    final mime = body.split(',').first.toLowerCase().trim();
    return mime.contains('mpegurl') || mime.contains('m3u8');
  }
  final lower = raw.toLowerCase();
  if (lower.contains('=m3u8')) return true;
  if (lower.contains('=mpd')) return false;
  for (final seg in uri.pathSegments) {
    if (seg.isEmpty) continue;
    if (seg.toLowerCase().contains('m3u8')) return true;
  }
  return false;
}

/// format 是否表示 HLS（规范见 Go `model.FormatIsHls`）。
bool kotvFormatIsHls(String? format) {
  switch (format?.trim().toLowerCase() ?? '') {
    case 'application/x-mpegurl':
    case 'application/vnd.apple.mpegurl':
    case 'audio/mpegurl':
    case 'audio/x-mpegurl':
    case 'hls':
    case 'm3u8':
      return true;
    default:
      return false;
  }
}

/// 这一次打开应按 HLS 处理：URL 强信号，或引擎/站源给出的 HLS format。
bool kotvIsHls(String url, String? format) {
  if (kotvUrlIsHls(url)) return true;
  return kotvFormatIsHls(format);
}

/// Exo 的 MediaItem mime。
///
/// - HLS（URL 强信号或 format）：锁 `application/x-mpegURL`（proxy 改写后无扩展名时必需）。
/// - 其它非空 format：原样交给 Media3。
/// - 空 format 且非 HLS：返回 null（不锁 mime；Media3 仅按 URI 选源）。
String? kotvPlayMime(String url, String? format) {
  if (kotvIsHls(url, format)) return 'application/x-mpegURL';
  final f = format?.trim() ?? '';
  if (f.isNotEmpty) return f;
  return null;
}

/// 错误文案是否像「主文件打开失败 / 未识别 m3u8」（无 prefix 时用，如 FVP）。
bool kotvHlsOpenFailedHint(String? text) {
  final t = (text ?? '').toLowerCase();
  return t.contains('avformat_open_input() failed') || t.contains('not detecting m3u8/hls');
}

/// media_kit：带 log prefix；vd/ad 解码器报错不算。
bool kotvMpvHlsOpenFailed(String prefix, String text) {
  final p = prefix.toLowerCase();
  if (p == 'vd' || p == 'ad') return false;
  return kotvHlsOpenFailedHint(text);
}

bool kotvMpvNetworkUrl(String url) {
  final scheme = Uri.tryParse(url.trim())?.scheme.toLowerCase() ?? '';
  return scheme == 'http' ||
      scheme == 'https' ||
      scheme == 'rtmp' ||
      scheme == 'rtsp' ||
      scheme == 'rtspt' ||
      scheme == 'proxy';
}

/// 短 EOF 是否值得强制 lavf HLS 再开一次（桌面 media_kit / 安卓原生 MPV 共用规则）。
///
/// 已强制 HLS、已再开过、直播、非网络、时长或进度超过 30s → 否。
/// 允许：URL 强信号，或开播时 format 为空（伪装源依赖 lavf，易误判成短片）。
/// 站源已声明非空且非 HLS 的 format → 否（避免短 progressive 误重试）。
bool kotvAllowHlsShortEofRetry({
  required bool hlsForced,
  required bool alreadyRetried,
  required bool live,
  required String url,
  String? format,
  required double durationSec,
  required double positionSec,
}) {
  if (hlsForced || alreadyRetried || live) return false;
  if (!kotvMpvNetworkUrl(url)) return false;
  if (durationSec > 30 || positionSec > 30) return false;
  if (kotvUrlIsHls(url)) return true;
  return (format ?? '').trim().isEmpty;
}
