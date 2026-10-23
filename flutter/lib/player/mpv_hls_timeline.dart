/// 地址按协议、路径和查询判断是不是 HLS。
bool kotvMpvUrlIsHls(String url) {
  final raw = url.trim();
  if (raw.isEmpty) return false;
  final uri = Uri.tryParse(raw);
  if (uri == null) return raw.contains('=m3u8');
  final scheme = uri.scheme.toLowerCase();
  if (scheme == 'rtsp' || scheme == 'rtspt') return false;
  if (scheme == 'data') {
    final body = raw.length > 5 ? raw.substring(5) : '';
    return !body.startsWith('application/dash+xml');
  }
  if (raw.contains('=m3u8')) return true;
  if (raw.contains('=mpd')) return false;
  final seg = _lastSegment(uri);
  if (seg == null) return false;
  if (seg.toLowerCase() == 'm3u8') return true;
  final dot = seg.lastIndexOf('.');
  if (dot < 0 || dot == seg.length - 1) return false;
  final ext = seg.substring(dot + 1).toLowerCase();
  return ext == 'm3u8' || ext == 'php';
}

/// 结尾斜杠会产生空段，取最后一段非空路径。
String? _lastSegment(Uri uri) {
  for (var i = uri.pathSegments.length - 1; i >= 0; i--) {
    final seg = uri.pathSegments[i];
    if (seg.isNotEmpty) return seg;
  }
  return null;
}

/// 这一次打开是 HLS。地址命中后，别的 mime 不能把它改回去。
/// mime 只有 `application/x-mpegURL` 算。
bool kotvIsHls(String url, String? format) {
  if (kotvMpvUrlIsHls(url)) return true;
  return (format?.trim().toLowerCase() ?? '') == 'application/x-mpegurl';
}

/// 交给播放器的 mime。是 HLS 时固定为 `application/x-mpegURL`。
String? kotvPlayMime(String url, String? format) {
  if (kotvIsHls(url, format)) return 'application/x-mpegURL';
  final f = format?.trim() ?? '';
  return f.isEmpty ? null : f;
}

/// 主文件打开失败的日志。解码器报错不算。
bool kotvMpvHlsOpenFailed(String prefix, String text) {
  final p = prefix.toLowerCase();
  if (p == 'vd' || p == 'ad') return false;
  final t = text.toLowerCase();
  return t.contains('avformat_open_input() failed') ||
      t.contains('failed to recognize file format');
}
