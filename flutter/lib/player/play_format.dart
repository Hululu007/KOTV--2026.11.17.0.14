import 'package:http/http.dart' as http;

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

/// 拉取点播 HLS 媒体单并加总 `#EXTINF`（毫秒）。直播 / 非 HLS → null。
/// 主清单时跟第一条 variant 再解析。供 media_kit / FVP 覆盖引擎偏短 demux 时长。
Future<int?> kotvFetchHlsVodDurationMs({
  required String url,
  required Map<String, String> headers,
  required bool live,
  String? format,
}) async {
  if (live || !kotvIsHls(url, format)) return null;
  Future<String?> getBody(String u) async {
    final res = await http
        .get(Uri.parse(u), headers: headers.isEmpty ? null : headers)
        .timeout(const Duration(seconds: 12));
    if (res.statusCode < 200 || res.statusCode >= 300) return null;
    return res.body;
  }

  var body = await getBody(url);
  if (body == null) return null;
  final direct = kotvHlsVodDurationMs(body);
  if (direct != null) return direct;
  final variant = kotvHlsFirstVariantUri(body);
  if (variant == null || variant.isEmpty) return null;
  final child = Uri.parse(url).resolve(variant).toString();
  body = await getBody(child);
  if (body == null) return null;
  return kotvHlsVodDurationMs(body);
}

/// 点播 HLS：节目单加总与引擎 demux 时长取较大者（FVP 等非 Media3 路径）。
/// Media3 / 桌面 libmpv 用引擎 `duration`/`duration/full`，不走此覆盖。
///
/// 仅当引擎时长已接近节目单时才用节目单拉长 UI。
Duration kotvPreferPlaylistDuration(Duration engine, Duration? playlist) {
  if (playlist == null || playlist <= Duration.zero) return engine;
  if (engine <= Duration.zero) return engine;
  if (playlist <= engine) return engine;
  // 引擎不足节目单 1/4 且不足 60s：视为仍按切片 demux，不用节目单掩盖。
  if (engine < const Duration(seconds: 60) &&
      engine.inMilliseconds * 4 < playlist.inMilliseconds) {
    return engine;
  }
  return playlist;
}

/// 媒体节目单正文是否为直播（有 EXTINF、无 ENDLIST、非主清单）。
bool kotvHlsBodyIsLiveMediaPlaylist(String body) {
  final text = body.trimLeft();
  if (!text.startsWith('#EXTM3U')) return false;
  var hasEnd = false;
  var hasStreamInf = false;
  var anyInf = false;
  for (final raw in text.split('\n')) {
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
    if (_extinfRe.hasMatch(line)) anyInf = true;
  }
  if (hasStreamInf || !anyInf) return false;
  return !hasEnd;
}

/// 拉取 HLS 正文判断是否直播清单（主清单跟首条 variant）。点播页据此升 live 会话。
Future<bool> kotvDetectHlsLivePlayback({
  required String url,
  required Map<String, String> headers,
  String? format,
}) async {
  if (!kotvIsHls(url, format)) return false;
  Future<String?> getBody(String u) async {
    final res = await http
        .get(Uri.parse(u), headers: headers.isEmpty ? null : headers)
        .timeout(const Duration(seconds: 12));
    if (res.statusCode < 200 || res.statusCode >= 300) return null;
    return res.body;
  }

  var body = await getBody(url);
  if (body == null) return false;
  if (kotvHlsBodyIsLiveMediaPlaylist(body)) return true;
  final variant = kotvHlsFirstVariantUri(body);
  if (variant == null || variant.isEmpty) return false;
  final child = Uri.parse(url).resolve(variant).toString();
  body = await getBody(child);
  if (body == null) return false;
  return kotvHlsBodyIsLiveMediaPlaylist(body);
}

/// 拉取点播 HLS 时长；[stillCurrent] 为 false 时丢弃过期结果（换源竞态）。
Future<Duration?> kotvRefreshHlsPlaylistDuration({
  required String url,
  required Map<String, String> headers,
  required bool live,
  required String? format,
  required bool Function() stillCurrent,
}) async {
  try {
    final ms = await kotvFetchHlsVodDurationMs(
      url: url,
      headers: headers,
      live: live,
      format: format,
    );
    if (ms == null || ms <= 0) return null;
    if (!stillCurrent()) return null;
    return Duration(milliseconds: ms);
  } catch (_) {
    return null;
  }
}

/// media_kit / FVP 共用：先清空节目单时长，拉取成功且仍为当前源再写回并通知。
Future<void> kotvApplyHlsPlaylistDurationRefresh({
  required String url,
  required Map<String, String> headers,
  required bool live,
  required String? format,
  required bool Function() stillCurrent,
  required void Function(Duration? value) setPlaylistDuration,
  required void Function() onUpdated,
}) async {
  setPlaylistDuration(null);
  final d = await kotvRefreshHlsPlaylistDuration(
    url: url,
    headers: headers,
    live: live,
    format: format,
    stillCurrent: stillCurrent,
  );
  if (d == null) return;
  setPlaylistDuration(d);
  onUpdated();
}
