/// URL 强信号快路径。规范见 Go `model.URLIsHls`；原生/Go 不能调 Dart，故各留副本。
/// 引擎 play 会正文嗅探写准 format；此处只补本地/失败再开。
bool kotvMpvUrlIsHls(String url) {
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

/// format 是否表示 HLS（与 Go `model.FormatIsHls` 同语义）。
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

/// 这一次打开是 HLS：URL 强信号，或引擎/站源给出的 HLS format。
/// 地址命中后，别的 mime 不能把它改回去。
bool kotvIsHls(String url, String? format) {
  if (kotvMpvUrlIsHls(url)) return true;
  return kotvFormatIsHls(format);
}

/// Exo 在 mime 非空时不看地址。只有空 mime 才按地址补成 HLS。
String? kotvPlayMime(String url, String? format) {
  final f = format?.trim() ?? '';
  if (f.isNotEmpty) {
    if (kotvFormatIsHls(f)) return 'application/x-mpegURL';
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
