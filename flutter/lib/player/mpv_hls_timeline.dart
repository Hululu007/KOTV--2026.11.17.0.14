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
  if (uri.pathSegments.isEmpty) return false;
  final seg = uri.pathSegments.last;
  if (seg.toLowerCase() == 'm3u8') return true;
  final dot = seg.lastIndexOf('.');
  if (dot < 0 || dot == seg.length - 1) return false;
  final ext = seg.substring(dot + 1).toLowerCase();
  return ext == 'm3u8' || ext == 'php';
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
