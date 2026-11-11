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

final _extinfRe = RegExp(r'^#EXTINF\s*:\s*([0-9.]+)', caseSensitive: false);
final _streamInfRe = RegExp(r'^#EXT-X-STREAM-INF\b', caseSensitive: false);

/// 点播媒体单：全部 `#EXTINF` 秒数之和（毫秒）。无 `#EXT-X-ENDLIST` 或非节目单 → null。
/// 主清单（含 `#EXT-X-STREAM-INF`）不加总，由调用方跟子 URI 后再解析。
int? kotvHlsVodDurationMs(String body) {
  final text = body.trimLeft();
  if (!text.startsWith('#EXTM3U')) return null;
  final lines = text.split('\n');
  var hasEnd = false;
  var hasStreamInf = false;
  var sumSec = 0.0;
  var anyInf = false;
  for (final raw in lines) {
    final line = raw.trim();
    if (line.isEmpty) continue;
    final upper = line.toUpperCase();
    if (upper.startsWith('#EXT-X-ENDLIST')) {
      hasEnd = true;
      continue;
    }
    if (_streamInfRe.hasMatch(line)) {
      hasStreamInf = true;
      continue;
    }
    final m = _extinfRe.firstMatch(line);
    if (m != null) {
      anyInf = true;
      sumSec += double.tryParse(m.group(1)!) ?? 0;
    }
  }
  if (!hasEnd || hasStreamInf || !anyInf) return null;
  if (sumSec <= 0) return null;
  return (sumSec * 1000).round();
}

/// 主清单时返回第一条子媒体 URI（相对路径保持原样，由调用方拼绝对地址）。
String? kotvHlsFirstVariantUri(String body) {
  final text = body.trimLeft();
  if (!text.startsWith('#EXTM3U')) return null;
  final lines = text.split('\n');
  var expectUri = false;
  for (final raw in lines) {
    final line = raw.trim();
    if (line.isEmpty) continue;
    if (_streamInfRe.hasMatch(line)) {
      expectUri = true;
      continue;
    }
    if (expectUri && !line.startsWith('#')) {
      return line;
    }
    if (!line.startsWith('#')) expectUri = false;
  }
  return null;
}
