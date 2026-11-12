/// 播放请求头规范化（全播放器共用）。
///
/// User-Agent 优先级：
/// 1. 请求头已有 User-Agent / ua
/// 2. 设置项 `ua`（非空）
/// 3. Android 上 [kotvSetPlatformPlayUa] 写入的系统 UA
/// 4. 桌面留空，由播放器自己带
/// 5. 其它平台用 [kotvDefaultPlayUA]
library;

import '../api/kotv_engine_url.dart';
import 'hls_js_support.dart';
import 'kotv_platform.dart';
import 'play_format.dart';

/// 输入 `o` 时填入的串。Android 尚未取到系统 UA 时也用它。
const kotvOkHttpPlayUA = 'okhttp/5.5.0';

const kotvDefaultPlayUA = kotvOkHttpPlayUA;

/// 输入 `c` 时填入的浏览器串。
const kotvChromePlayUA =
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/155.0.0.0 Safari/537.36';

String _settingsUa = '';
String _platformPlayUa = '';

/// 用设置里的 `ua` 刷新缓存（开播前 / 设置变更后调用）。
void kotvApplyPlayUaSetting(String? raw) {
  _settingsUa = (raw ?? '').trim();
}

/// Android 启动时写入 `Util.getUserAgent` 的结果。
void kotvSetPlatformPlayUa(String? raw) {
  _platformPlayUa = (raw ?? '').trim();
}

/// 当前生效的缺省 UA。设置优先。桌面在设置留空时返回空串。
String kotvEffectivePlayUa() {
  if (_settingsUa.isNotEmpty) return _settingsUa;
  if (_platformPlayUa.isNotEmpty) return _platformPlayUa;
  if (kotvIsDesktop()) return '';
  return kotvDefaultPlayUA;
}

/// 引擎临时代理 HTTP（id 会过期）。口径同 Go [playproxy.IsEphemeralURL]。
bool kotvIsEphemeralPlayUrl(String url) {
  final u = url.toLowerCase().trim();
  return u.contains('/proxy/play?') ||
      u.contains('/proxy/cached_m3u8') ||
      u.contains('/proxy/rolling_m3u8') ||
      u.contains('/proxy/hls/') ||
      u.contains('/proxy/bt/');
}

/// 本地/引擎代理地址：临时代理、爬虫 `/proxy`，不能再带 CDN 头。
bool kotvIsLocalProxyUrl(String url) {
  if (kotvIsEphemeralPlayUrl(url)) return true;
  final u = url.toLowerCase();
  if (u.contains('/proxy/play')) return true;
  try {
    final uri = Uri.parse(url);
    var path = kotvStripMediaKey(uri.path);
    if (path.endsWith('/') && path.length > 1) {
      path = path.substring(0, path.length - 1);
    }
    if (path == '/proxy') return true;
  } catch (_) {}
  return u.contains('127.0.0.1:') && u.contains('/proxy');
}

/// 这次打开能否把请求头交给播放器。为真且直链可连时，打开站点地址。
///
/// - Exo、FVP、MPV：列表和分片走各自的 HTTP 栈，整份头带上。
/// - html / art / 西瓜 / 全能：只有 HLS 且 hls.js 可用时由 xhr 带上。
bool kotvPlaybackSendsHeaders({
  required String playerVal,
  required String mediaUrl,
  String? format,
  bool? hlsJsSupported,
}) {
  switch (playerVal) {
    case 'innie#exo':
    case 'innie#fvp':
    case 'innie#mpv':
      return true;
    case 'innie#html':
    case 'innie#art':
    case 'innie#xg':
    case 'innie#zw':
      if (!(hlsJsSupported ?? kotvWebHlsJsSupported())) return false;
      return kotvIsHls(mediaUrl, format);
    default:
      return false;
  }
}

/// 播放器能否自己在每次请求上带请求头。只有这时才打开 [directUrl]。
bool kotvCanOpenDirect(String url) {
  final u = url.trim();
  if (u.isEmpty) return false;
  final uri = Uri.tryParse(u);
  if (uri == null) return false;
  if (uri.scheme != 'http' && uri.scheme != 'https') return false;
  final host = uri.host.toLowerCase();
  if (host.isEmpty || host == 'localhost' || host == '127.0.0.1' || host == '::1') {
    return false;
  }
  if (kotvIsLocalProxyUrl(u)) return false;
  return true;
}

/// 是否优先打开 Direct（CDN 原址）。
///
/// Exo 始终优先（播放器内广告/sync）；其它内置播放器仅在去广告关闭时优先。
bool kotvShouldPreferPlayDirect({
  required String playerVal,
  required bool adFilterOn,
}) {
  final p = playerVal.trim();
  if (!p.startsWith('innie#')) return false;
  if (p == 'innie#exo') return true;
  return !adFilterOn;
}

/// 直连不可播时，是否改开引擎已下发的透传 [playUrl]（不再打 API）。
///
/// Exo 不走；已在本地代理、已切过、或 [playUrl] 不是本地代理 URL → 否。
bool kotvShouldSwitchToProxyAfterOpenFailure({
  required String playerVal,
  required String openUrl,
  required String playUrl,
  required bool alreadySwitched,
}) {
  if (alreadySwitched) return false;
  final p = playerVal.trim();
  if (p == 'innie#exo' || !p.startsWith('innie#')) return false;
  if (kotvIsLocalProxyUrl(openUrl)) return false;
  return kotvIsLocalProxyUrl(playUrl);
}

/// 选定最终 open 地址。
/// [preferDirect] 且 [sendHeaders]、[directUrl] 可直连时，打开站点地址。
/// 其它情况打开引擎返回的播放地址。
({String url, Map<String, String>? headers}) kotvResolvePlayOpenTarget({
  required String playUrl,
  required Map<String, String> headers,
  required bool magnet,
  String directUrl = '',
  bool sendHeaders = false,
  bool preferDirect = false,
}) {
  if (magnet) {
    return (url: playUrl, headers: headers.isEmpty ? null : headers);
  }
  if (preferDirect && sendHeaders && kotvCanOpenDirect(directUrl)) {
    return (url: directUrl, headers: headers.isEmpty ? null : headers);
  }
  final proxied = kotvIsLocalProxyUrl(playUrl);
  return (
    url: playUrl,
    headers: proxied || headers.isEmpty ? null : headers,
  );
}

/// 详情/直播共用：按去广告与播放器能力选 Direct 或引擎 url，并规范化请求头。
({String url, Map<String, String>? headers}) kotvResolvePreferredPlayOpen({
  required String playerVal,
  required String playUrl,
  required String directUrl,
  required Map<String, String> rawHeaders,
  required bool magnet,
  String? format,
  required bool adFilterOn,
  bool forceProxy = false,
}) {
  final preferDirect = !forceProxy &&
      kotvShouldPreferPlayDirect(playerVal: playerVal, adFilterOn: adFilterOn);
  final mediaHint =
      preferDirect && directUrl.trim().isNotEmpty ? directUrl : playUrl;
  return kotvResolvePlayOpenTarget(
    playUrl: playUrl,
    directUrl: directUrl,
    headers: kotvNormalizePlayHeaders(rawHeaders, url: mediaHint),
    magnet: magnet,
    sendHeaders: kotvPlaybackSendsHeaders(
      playerVal: playerVal,
      mediaUrl: mediaHint,
      format: format,
    ),
    preferDirect: preferDirect,
  );
}

/// 规范化请求头；本地代理 URL 返回空（代理侧已带远端头）。
Map<String, String> kotvNormalizePlayHeaders(
  Map<String, String>? headers, {
  required String url,
  bool addDefaultUa = true,
}) {
  if (kotvIsLocalProxyUrl(url)) return const {};
  final out = <String, String>{};
  if (headers != null) {
    for (final e in headers.entries) {
      var k = e.key.trim();
      var v = e.value.trim();
      if (k.isEmpty || v.isEmpty) continue;
      // 嗅探/续传残留的 Range 会导致整段开播只拿片段。
      if (k.toLowerCase() == 'range') continue;
      if (v.contains(r'$') && (v.contains('#') || k.toLowerCase() == 'header')) {
        for (final part in v.split('#')) {
          final i = part.indexOf(r'$');
          if (i <= 0) continue;
          final hk = _canonicalHeader(part.substring(0, i).trim());
          final hv = part.substring(i + 1).trim();
          if (hk.isEmpty || hv.isEmpty) continue;
          if (hk.toLowerCase() == 'range') continue;
          out[hk] = hv;
        }
        continue;
      }
      out[_canonicalHeader(k)] = v;
    }
  }
  if (addDefaultUa && !out.keys.any((k) => k.toLowerCase() == 'user-agent')) {
    final ua = kotvEffectivePlayUa();
    if (ua.isNotEmpty) out['User-Agent'] = ua;
  }
  return out;
}

/// User-Agent、Referer 走播放器独立属性，其余交给 `http-header-fields`。
class KotvMpvHeaderSplit {
  const KotvMpvHeaderSplit({
    required this.userAgent,
    required this.referer,
    required this.fields,
  });

  final String userAgent;
  final String referer;
  final Map<String, String> fields;
}

KotvMpvHeaderSplit kotvSplitMpvHeaders(Map<String, String> headers) {
  var ua = '';
  var referer = '';
  final fields = <String, String>{};
  for (final e in headers.entries) {
    final k = e.key.trim();
    final v = e.value.trim();
    if (k.isEmpty || v.isEmpty) continue;
    switch (k.toLowerCase()) {
      case 'user-agent':
      case 'ua':
        ua = v;
        break;
      case 'referer':
      case 'referrer':
        referer = v;
        break;
      case 'range':
        break;
      default:
        fields[k] = v;
    }
  }
  return KotvMpvHeaderSplit(userAgent: ua, referer: referer, fields: fields);
}

String _canonicalHeader(String k) {
  switch (k.toLowerCase()) {
    case 'user-agent':
    case 'ua':
      return 'User-Agent';
    case 'referer':
    case 'referrer':
      return 'Referer';
    case 'cookie':
      return 'Cookie';
    case 'origin':
      return 'Origin';
    case 'host':
      return 'Host';
    default:
      return k;
  }
}
