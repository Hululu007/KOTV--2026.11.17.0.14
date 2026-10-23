import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

const _headBytes = 64 * 1024;

/// 跳过 BOM 和空白后：FLV / TS 不是整段 HLS。
/// `#EXTM3U` 且含分片或 EXT-X 标签才是。
bool kotvMpvHeadIsHlsTimeline(List<int> bytes) {
  if (bytes.isEmpty) return false;
  var i = 0;
  if (bytes.length >= 3 && bytes[0] == 0xEF && bytes[1] == 0xBB && bytes[2] == 0xBF) {
    i = 3;
  }
  while (i < bytes.length) {
    final b = bytes[i];
    if (b == 0x20 || b == 0x09 || b == 0x0A || b == 0x0D) {
      i++;
      continue;
    }
    break;
  }
  if (i >= bytes.length) return false;
  if (bytes[i] == 0x47) return false;
  if (_asciiAt(bytes, i, 'FLV')) return false;
  if (!_asciiAt(bytes, i, '#EXTM3U')) return false;
  final text = latin1.decode(bytes, allowInvalid: true).toUpperCase();
  return text.contains('#EXTINF') ||
      text.contains('#EXT-X-STREAM-INF') ||
      text.contains('#EXT-X-TARGETDURATION') ||
      text.contains('#EXT-X-MEDIA-SEQUENCE');
}

bool kotvMpvHlsForceRejected(String prefix, String text) {
  final t = text.toLowerCase();
  if (t.contains('avformat_open_input() failed')) return true;
  if (t.contains('failed to recognize file format')) return true;
  final p = prefix.toLowerCase();
  if (p == 'lavf' || p == 'demux') {
    return t.contains('failed') && t.contains('open');
  }
  return false;
}

Future<bool> kotvMpvHlsTimeline(String url, Map<String, String> headers) async {
  final bytes = await _readPlayHead(url, headers);
  if (bytes == null || bytes.isEmpty) return false;
  return kotvMpvHeadIsHlsTimeline(bytes);
}

Future<List<int>?> _readPlayHead(String url, Map<String, String> headers) async {
  final uri = Uri.tryParse(url.trim());
  if (uri == null || (uri.scheme != 'http' && uri.scheme != 'https')) return null;
  final client = http.Client();
  try {
    final req = http.Request('GET', uri);
    headers.forEach((k, v) {
      if (k.trim().isEmpty || v.trim().isEmpty) return;
      if (k.trim().toLowerCase() == 'range') return;
      req.headers[k] = v;
    });
    final res = await client.send(req).timeout(const Duration(seconds: 8));
    if (res.statusCode >= 400) {
      await res.stream.drain<void>();
      return null;
    }
    final out = <int>[];
    await for (final chunk in res.stream.timeout(const Duration(seconds: 8))) {
      out.addAll(chunk);
      if (out.length >= _headBytes) break;
    }
    if (out.length > _headBytes) return out.sublist(0, _headBytes);
    return out;
  } catch (_) {
    return null;
  } finally {
    client.close();
  }
}

bool _asciiAt(List<int> bytes, int start, String text) {
  if (start + text.length > bytes.length) return false;
  for (var n = 0; n < text.length; n++) {
    if (bytes[start + n] != text.codeUnitAt(n)) return false;
  }
  return true;
}
