import 'dart:io';

import 'play_headers.dart';
import 'play_redirect_types.dart';

/// 播放器缺省 UA。请求头里没有时，探测跳转用这个，避免和随后的打开不一致。
const _playerUa = 'libmpv';

/// libcurl 会跟 302，但顶层解复用仍用打开时的地址拼相对列表。
/// 先换成最终地址再打开。本地代理自己跟上游，这里不再请求。
Future<KotvFollowedPlay> kotvFollowPlayUrl(
  String raw,
  Map<String, String> headers,
) async {
  final original = raw.trim();
  final out = Map<String, String>.from(headers);
  if (!_http(original) || kotvIsLocalProxyUrl(original)) {
    return KotvFollowedPlay(url: original, headers: out, moved: false);
  }
  var current = original;
  final cookies = <String, String>{};
  for (var hop = 0; hop < 5; hop++) {
    final client = HttpClient();
    client.connectionTimeout = const Duration(seconds: 8);
    client.autoUncompress = false;
    try {
      final uri = Uri.parse(current);
      final req = await client.getUrl(uri).timeout(const Duration(seconds: 8));
      req.followRedirects = false;
      _applyProbeHeaders(req, out, cookies);
      final resp = await req.close().timeout(const Duration(seconds: 8));
      _collectCookies(resp, cookies);
      final code = resp.statusCode;
      final loc = resp.headers.value(HttpHeaders.locationHeader)?.trim() ?? '';
      await _dropBody(resp);
      if (code < 300 || code >= 400) return _finish(original, current, cookies, out);
      if (loc.isEmpty) return _finish(original, current, cookies, out);
      final next = _resolveRedirect(current, loc);
      if (!_http(next) || next == current) {
        return _finish(original, current, cookies, out);
      }
      if (_authorityChanged(current, next)) {
        out.removeWhere((k, _) => k.toLowerCase() == 'authorization');
      }
      current = next;
    } catch (_) {
      return current != original
          ? _finish(original, current, cookies, out)
          : KotvFollowedPlay(url: original, headers: out, moved: false);
    } finally {
      client.close(force: true);
    }
  }
  return _finish(original, current, cookies, out);
}

bool _http(String url) =>
    url.startsWith('http://') || url.startsWith('https://');

KotvFollowedPlay _finish(
  String original,
  String current,
  Map<String, String> cookies,
  Map<String, String> headers,
) {
  if (cookies.isNotEmpty &&
      !headers.keys.any((k) => k.toLowerCase() == 'cookie')) {
    headers['Cookie'] =
        cookies.entries.map((e) => '${e.key}=${e.value}').join('; ');
  }
  return KotvFollowedPlay(
    url: current,
    headers: headers,
    moved: current != original,
  );
}

void _applyProbeHeaders(
  HttpClientRequest req,
  Map<String, String> headers,
  Map<String, String> cookies,
) {
  var ua = '';
  for (final e in headers.entries) {
    final key = e.key.trim();
    final value = e.value.trim();
    if (key.isEmpty || value.isEmpty) continue;
    final lk = key.toLowerCase();
    if (lk == 'range' || lk == 'host') continue;
    if (lk == 'user-agent' || lk == 'ua') {
      ua = value;
      continue;
    }
    if (lk == 'cookie' && cookies.isNotEmpty) continue;
    try {
      req.headers.set(key, value, preserveHeaderCase: true);
    } catch (_) {}
  }
  if (cookies.isNotEmpty) {
    try {
      req.headers.set(
        'Cookie',
        cookies.entries.map((e) => '${e.key}=${e.value}').join('; '),
        preserveHeaderCase: true,
      );
    } catch (_) {}
  }
  if (ua.isEmpty) ua = _playerUa;
  try {
    req.headers.set(HttpHeaders.userAgentHeader, ua);
  } catch (_) {}
}

void _collectCookies(HttpClientResponse resp, Map<String, String> out) {
  final values = resp.headers[HttpHeaders.setCookieHeader];
  if (values == null) return;
  for (final raw in values) {
    final pair = raw.split(';').first.trim();
    final eq = pair.indexOf('=');
    if (eq <= 0) continue;
    out[pair.substring(0, eq).trim()] = pair.substring(eq + 1).trim();
  }
}

bool _authorityChanged(String from, String to) {
  final a = Uri.tryParse(from);
  final b = Uri.tryParse(to);
  if (a == null || b == null) return true;
  if (a.scheme.toLowerCase() != b.scheme.toLowerCase()) return true;
  if (a.host.toLowerCase() != b.host.toLowerCase()) return true;
  final ap = a.hasPort ? a.port : -1;
  final bp = b.hasPort ? b.port : -1;
  return ap != bp;
}

String _resolveRedirect(String current, String location) {
  final base = Uri.tryParse(current);
  if (base == null) return location;
  try {
    return base.resolve(location).toString();
  } catch (_) {
    return location;
  }
}

Future<void> _dropBody(HttpClientResponse resp) async {
  try {
    final sock = await resp.detachSocket();
    sock.destroy();
  } catch (_) {}
}
