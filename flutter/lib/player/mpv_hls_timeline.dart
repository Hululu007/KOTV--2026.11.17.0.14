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

/// 这一次打开要指定 lavf 的 hls。地址或格式命中后，失败不再改回探测。
bool kotvMpvHlsLavf(String url, String? format) {
  if (kotvMpvUrlIsHls(url)) return true;
  final f = format?.trim().toLowerCase() ?? '';
  return f == 'application/x-mpegurl' ||
      f == 'application/vnd.apple.mpegurl' ||
      f == 'hls' ||
      f == 'm3u8';
}

/// 主文件打开失败的日志。解码器报错不算。
bool kotvMpvHlsOpenFailed(String prefix, String text) {
  final p = prefix.toLowerCase();
  if (p == 'vd' || p == 'ad') return false;
  final t = text.toLowerCase();
  return t.contains('avformat_open_input() failed') ||
      t.contains('failed to recognize file format');
}
