/// 地址按路径和查询判断是不是 HLS。
bool kotvMpvUrlIsHls(String url) {
  final raw = url.trim();
  if (raw.isEmpty) return false;
  if (raw.contains('=m3u8')) return true;
  final uri = Uri.tryParse(raw);
  if (uri == null) return false;
  if (uri.scheme.toLowerCase() == 'data') {
    final body = raw.length > 5 ? raw.substring(5) : '';
    return !body.startsWith('application/dash+xml');
  }
  if (uri.pathSegments.isEmpty) return false;
  final seg = uri.pathSegments.last;
  if (seg.toLowerCase() == 'm3u8') return true;
  final dot = seg.lastIndexOf('.');
  if (dot < 0 || dot == seg.length - 1) return false;
  final ext = seg.substring(dot + 1).toLowerCase();
  return ext == 'm3u8' || ext == 'php';
}

/// 格式字段已经是 mpegURL。这种打开失败后不再重试。
bool kotvMpvFormatIsMpegUrl(String? format) {
  return format?.trim().toLowerCase() == 'application/x-mpegurl';
}

/// 这一次打开要指定 lavf 的 hls。
bool kotvMpvHlsLavf(String url, String? format) {
  if (kotvMpvUrlIsHls(url)) return true;
  final f = format?.trim().toLowerCase() ?? '';
  return f == 'application/x-mpegurl' || f == 'hls' || f == 'm3u8';
}

/// 打开失败、可以再按 HLS 试一次的日志。
bool kotvMpvHlsOpenFailed(String prefix, String text) {
  final t = text.toLowerCase();
  if (t.contains('avformat_open_input() failed')) return true;
  if (t.contains('failed to recognize file format')) return true;
  final p = prefix.toLowerCase();
  if (p == 'lavf' || p == 'demux') {
    return t.contains('failed') && t.contains('open');
  }
  return false;
}
