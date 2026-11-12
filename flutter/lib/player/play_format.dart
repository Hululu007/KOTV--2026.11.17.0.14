/// 播放容器判定（各播放器共用）。
///
/// 正文探测在引擎 `planPlayback`；客户端以回包 format 为主，URL 仅强信号快路径。
/// Dart 不能调 Go，URL/format 规则在此与 `model.URLIsHls` / `FormatIsHls` 各留一份副本。

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

/// Exo / 安卓原生 MPV / 桌面 MPV 开播 mime。
///
/// - HLS（URL 强信号或 format）：锁 `application/x-mpegURL`（proxy 改写后无扩展名时必需）。
/// - 其它非空 format：原样交给引擎。
/// - 空 format 且非 HLS：返回 null（不锁 mime；引擎仅按 URI 选源）。
String? kotvPlayMime(String url, String? format) {
  if (kotvIsHls(url, format)) return 'application/x-mpegURL';
  final f = format?.trim() ?? '';
  if (f.isNotEmpty) return f;
  return null;
}

/// 桌面 / 安卓 MPV：尚未按 HLS 处理，且错误文案指向网络/IO 失败时，锁 HLS 再开一次。
/// 安卓同判定在 KotvAndroidPlayback.shouldRetryMpvHlsAfterError。
bool kotvShouldRetryMpvHlsAfterError({
  required String url,
  required String? format,
  required bool alreadyRetried,
  required String errorText,
}) {
  if (alreadyRetried) return false;
  if (kotvIsHls(url, format)) return false;
  final t = errorText.toLowerCase();
  if (t.contains('tcp:') || (t.contains('http') && t.contains('error'))) {
    return true;
  }
  if (t.contains('failed to open') || t.contains('error opening') || t.contains('i/o')) {
    return true;
  }
  return false;
}
