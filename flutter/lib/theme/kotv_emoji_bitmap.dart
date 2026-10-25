import 'dart:typed_data';

import 'package:flutter/services.dart';

/// 从内嵌 Noto Color Emoji（CBDT）里取出彩色 PNG。
/// iOS 模拟器打不开 Apple Color Emoji，Skia 也画不出这套点阵，只能自己取图。
class KotvEmojiBitmap {
  KotvEmojiBitmap._(this._bytes, this._ranges, this._strikes, this._liga);

  final Uint8List _bytes;
  final List<(int, int, int)> _ranges;
  final List<_Strike> _strikes;
  final Map<int, List<_Liga>> _liga;

  static KotvEmojiBitmap? _cached;
  static Future<KotvEmojiBitmap>? _loading;

  static Future<KotvEmojiBitmap> load() {
    final cached = _cached;
    if (cached != null) return Future.value(cached);
    return _loading ??= () async {
      final data = await rootBundle.load('assets/fonts/NotoColorEmoji.ttf');
      final parsed = KotvEmojiBitmap.parse(data.buffer.asUint8List());
      _cached = parsed;
      return parsed;
    }();
  }

  static KotvEmojiBitmap parse(Uint8List bytes) {
    final d = ByteData.sublistView(bytes);
    int table(String tag) {
      final n = d.getUint16(4);
      for (var i = 0; i < n; i++) {
        final o = 12 + i * 16;
        final t = String.fromCharCodes(bytes.sublist(o, o + 4));
        if (t == tag) return d.getUint32(o + 8);
      }
      throw StateError(tag);
    }

    final ranges = <(int, int, int)>[];
    final cmap = table('cmap');
    final ntab = d.getUint16(cmap + 2);
    for (var i = 0; i < ntab; i++) {
      final o = d.getUint32(cmap + 4 + i * 8 + 4);
      if (d.getUint16(cmap + o) != 12) continue;
      final start = cmap + o;
      final groups = d.getUint32(start + 12);
      var p = start + 16;
      for (var g = 0; g < groups; g++) {
        ranges.add((
          d.getUint32(p),
          d.getUint32(p + 4),
          d.getUint32(p + 8),
        ));
        p += 12;
      }
    }

    final cbdt = table('CBDT');
    final cblc = table('CBLC');
    final indexOff = d.getUint32(cblc + 8);
    final nsub = d.getUint32(cblc + 16);
    final array = cblc + indexOff;
    final strikes = <_Strike>[];
    for (var i = 0; i < nsub; i++) {
      final rec = array + i * 8;
      final first = d.getUint16(rec);
      final last = d.getUint16(rec + 2);
      final extra = d.getUint32(rec + 4);
      final sub = array + extra;
      final indexFormat = d.getUint16(sub);
      final imageFormat = d.getUint16(sub + 2);
      final dataOff = d.getUint32(sub + 4);
      if (indexFormat != 1 || imageFormat != 17) continue;
      strikes.add(_Strike(first, last, cbdt + dataOff, sub + 8));
    }

    final liga = <int, List<_Liga>>{};
    final gsub = table('GSUB');
    final lookupList = gsub + d.getUint16(gsub + 8);
    final lookupCount = d.getUint16(lookupList);
    for (var i = 0; i < lookupCount; i++) {
      final lookup = lookupList + d.getUint16(lookupList + 2 + i * 2);
      if (d.getUint16(lookup) != 4) continue;
      final subCount = d.getUint16(lookup + 4);
      for (var s = 0; s < subCount; s++) {
        final sub = lookup + d.getUint16(lookup + 6 + s * 2);
        if (d.getUint16(sub) != 1) continue;
        final coverage = sub + d.getUint16(sub + 2);
        final glyphs = _coverage(d, coverage);
        final setCount = d.getUint16(sub + 4);
        for (var k = 0; k < setCount && k < glyphs.length; k++) {
          final set = sub + d.getUint16(sub + 6 + k * 2);
          final ligCount = d.getUint16(set);
          final first = glyphs[k];
          final list = liga.putIfAbsent(first, () => []);
          for (var n = 0; n < ligCount; n++) {
            final lig = set + d.getUint16(set + 2 + n * 2);
            final out = d.getUint16(lig);
            final comp = d.getUint16(lig + 2);
            final rest = <int>[];
            for (var c = 0; c < comp - 1; c++) {
              rest.add(d.getUint16(lig + 4 + c * 2));
            }
            list.add(_Liga(rest, out));
          }
        }
      }
    }
    for (final list in liga.values) {
      list.sort((a, b) => b.rest.length.compareTo(a.rest.length));
    }
    return KotvEmojiBitmap._(bytes, ranges, strikes, liga);
  }

  int? glyphOf(int cp) {
    for (final r in _ranges) {
      if (cp >= r.$1 && cp <= r.$2) return r.$3 + (cp - r.$1);
    }
    return null;
  }

  List<Uint8List> pngsFor(String text) {
    final glyphs = <int>[];
    for (final cp in text.runes) {
      if (cp == 0xFE0F || cp == 0xFE0E) continue;
      final g = glyphOf(cp);
      if (g != null) glyphs.add(g);
    }
    final out = <Uint8List>[];
    var i = 0;
    while (i < glyphs.length) {
      final ligs = _liga[glyphs[i]];
      var used = 1;
      var glyph = glyphs[i];
      if (ligs != null) {
        for (final lig in ligs) {
          if (i + 1 + lig.rest.length > glyphs.length) continue;
          var ok = true;
          for (var k = 0; k < lig.rest.length; k++) {
            if (glyphs[i + 1 + k] != lig.rest[k]) {
              ok = false;
              break;
            }
          }
          if (!ok) continue;
          glyph = lig.out;
          used = 1 + lig.rest.length;
          break;
        }
      }
      final png = _png(glyph);
      if (png != null) out.add(png);
      i += used;
    }
    return out;
  }

  Uint8List? _png(int gid) {
    final d = ByteData.sublistView(_bytes);
    for (final s in _strikes) {
      if (gid < s.first || gid > s.last) continue;
      final slot = s.offsets + (gid - s.first) * 4;
      final a = d.getUint32(slot);
      final abs = s.data + a;
      final len = d.getUint32(abs + 5);
      return Uint8List.sublistView(_bytes, abs + 9, abs + 9 + len);
    }
    return null;
  }
}

class _Strike {
  const _Strike(this.first, this.last, this.data, this.offsets);
  final int first;
  final int last;
  final int data;
  final int offsets;
}

class _Liga {
  const _Liga(this.rest, this.out);
  final List<int> rest;
  final int out;
}

List<int> _coverage(ByteData d, int pos) {
  final format = d.getUint16(pos);
  if (format == 1) {
    final n = d.getUint16(pos + 2);
    return [for (var i = 0; i < n; i++) d.getUint16(pos + 4 + i * 2)];
  }
  if (format == 2) {
    final n = d.getUint16(pos + 2);
    final out = <int>[];
    for (var i = 0; i < n; i++) {
      final rec = pos + 4 + i * 6;
      final start = d.getUint16(rec);
      final end = d.getUint16(rec + 2);
      for (var g = start; g <= end; g++) {
        out.add(g);
      }
    }
    return out;
  }
  return const [];
}
