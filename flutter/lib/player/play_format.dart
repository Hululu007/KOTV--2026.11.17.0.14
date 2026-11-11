/// 播放容器判定（各播放器共用）。
///
/// 规范见 Go `model.URLIsHls` / `model.FormatIsHls`；引擎 `planPlayback` 会正文嗅探写准 format。
/// Dart / Kotlin 不能调 Go，故各留 URL 副本；客户端以引擎 format 为主，URL 仅强信号快路径。

/// URL 强信号快路径（与 Go `URLIsHls` 同语义）。
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

/// format 是否表示 HLS（与 Go `FormatIsHls` 同语义）。
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
/// - 空 format 且非 HLS：返回 null，由 Media3 自嗅。
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
