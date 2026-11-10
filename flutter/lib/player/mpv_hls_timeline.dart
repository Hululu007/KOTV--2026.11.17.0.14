/// 地址按协议、路径和查询判断是不是 HLS。
/// 安卓原生 MPV 片尾/加载失败重开用同一套规则，见 `KotvMpvHls.kt`。
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
  // 任一路径段含 m3u8（getM3u8、/m3u8/xxx）；仅查询参数里的 id=2.m3u8 不算。
  for (final seg in uri.pathSegments) {
    if (seg.isEmpty) continue;
    final lower = seg.toLowerCase();
    if (lower == 'm3u8' || lower.contains('m3u8')) return true;
    final dot = lower.lastIndexOf('.');
    if (dot >= 0 && dot < lower.length - 1) {
      final ext = lower.substring(dot + 1);
      if (ext == 'm3u8' || ext == 'php') return true;
    }
  }
  return false;
}

/// 这一次打开是 HLS。地址命中后，别的 mime 不能把它改回去。
/// mime 只有 `application/x-mpegURL` 算。
bool kotvIsHls(String url, String? format) {
  if (kotvMpvUrlIsHls(url)) return true;
  return (format?.trim().toLowerCase() ?? '') == 'application/x-mpegurl';
}

/// Exo 在 mime 非空时不看地址。只有空 mime 才按地址补成 HLS。
String? kotvPlayMime(String url, String? format) {
  final f = format?.trim() ?? '';
  if (f.isNotEmpty) {
    if (f.toLowerCase() == 'application/x-mpegurl') return 'application/x-mpegURL';
    return f;
  }
  if (kotvMpvUrlIsHls(url)) return 'application/x-mpegURL';
  return null;
}

/// 主文件打开失败，或 lavf 明确说认不出非标准后缀/MIME 的 m3u8：应强制 demuxer-lavf-format=hls 重开。
/// 解码器报错不算。
bool kotvMpvHlsOpenFailed(String prefix, String text) {
  final p = prefix.toLowerCase();
  if (p == 'vd' || p == 'ad') return false;
  final t = text.toLowerCase();
  if (t.contains('avformat_open_input() failed')) return true;
  if (t.contains('not detecting m3u8/hls')) return true;
  return false;
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
