import 'dart:async';
import 'dart:typed_data';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';

import '../util/kotv_io.dart';
import 'kotv_emoji_bitmap.dart';

/// 彩色表情。iOS / macOS 的 Skia 画不出 Apple Color Emoji，改用内嵌 CBDT。
bool kotvRuneIsEmoji(int rune) {
  if (rune == 0x200D || rune == 0xFE0F || rune == 0xFE0E || rune == 0x20E3) {
    return true;
  }
  if (rune >= 0x1F3FB && rune <= 0x1F3FF) return true;
  if (rune >= 0x1F1E6 && rune <= 0x1F1FF) return true;
  if (rune >= 0x1F000 && rune <= 0x1FAFF) return true;
  if (rune >= 0x2600 && rune <= 0x27BF) return true;
  if (rune >= 0x2190 && rune <= 0x21FF) return false;
  return false;
}

bool kotvStringHasEmoji(String text) {
  for (final rune in text.runes) {
    if (kotvRuneIsEmoji(rune)) return true;
  }
  return false;
}

/// iOS / macOS 的 Apple Color Emoji 没有粗体，Skia 也画不出 sbix。
/// 用内嵌的 Noto Color Emoji（CBDT）单独画表情。
String? kotvEmojiFamily() {
  if (kIsWeb) return null;
  if (Platform.isIOS || Platform.isMacOS) return 'NotoColorEmoji';
  if (Platform.isWindows) return 'Segoe UI Emoji';
  if (Platform.isAndroid || Platform.isLinux) return 'Noto Color Emoji';
  return null;
}

TextStyle kotvEmojiRunStyle(TextStyle style) {
  final family = kotvEmojiFamily();
  return style.copyWith(
    fontFamily: family,
    fontFamilyFallback: family == null ? null : <String>[family],
    fontWeight: FontWeight.w400,
    fontStyle: FontStyle.normal,
  );
}

bool kotvRuneIsSymbol(int rune) {
  if (rune >= 0x2500 && rune <= 0x257F) return true;
  return rune == 0x2039 || rune == 0x203A;
}

TextStyle kotvSymbolRunStyle(TextStyle style) {
  return style.copyWith(
    fontFamily: 'Menlo',
    fontFamilyFallback: const ['Apple Symbols', 'Menlo'],
    fontWeight: FontWeight.w400,
  );
}

InlineSpan kotvEmojiSpan(String text, TextStyle style) {
  if (!kotvStringHasEmoji(text) && !text.runes.any(kotvRuneIsSymbol)) {
    return TextSpan(text: text, style: style);
  }
  final emojiStyle = kotvEmojiRunStyle(style);
  final symbolStyle = kotvSymbolRunStyle(style);
  final spans = <InlineSpan>[];
  final buf = StringBuffer();
  var kind = 0;
  void flush(int k) {
    if (buf.isEmpty) return;
    var s = buf.toString();
    buf.clear();
    if (k == 1) {
      s = s.replaceAll('\uFE0F', '').replaceAll('\uFE0E', '');
      if (s.isEmpty) return;
    }
    spans.add(TextSpan(
      text: s,
      style: k == 1 ? emojiStyle : (k == 2 ? symbolStyle : style),
    ));
  }

  for (final rune in text.runes) {
    final k = kotvRuneIsEmoji(rune) ? 1 : (kotvRuneIsSymbol(rune) ? 2 : 0);
    if (buf.isNotEmpty && k != kind) flush(kind);
    kind = k;
    buf.writeCharCode(rune);
  }
  flush(kind);
  return TextSpan(style: style, children: spans);
}

class KotvEmojiText extends StatelessWidget {
  const KotvEmojiText(
    this.text, {
    super.key,
    required this.style,
    this.maxLines = 1,
    this.overflow = TextOverflow.ellipsis,
    this.textAlign,
    this.softWrap = false,
  });

  final String text;
  final TextStyle style;
  final int? maxLines;
  final TextOverflow overflow;
  final TextAlign? textAlign;
  final bool softWrap;

  @override
  Widget build(BuildContext context) {
    final span = (!kIsWeb && Platform.isIOS && kotvStringHasEmoji(text))
        ? _iosEmojiSpan(text, style)
        : kotvEmojiSpan(text, style);
    return Text.rich(
      span,
      maxLines: maxLines,
      overflow: overflow,
      textAlign: textAlign,
      softWrap: softWrap,
    );
  }
}

final Map<String, List<Uint8List>> _emojiPngCache = {};

InlineSpan _iosEmojiSpan(String text, TextStyle style) {
  final symbolStyle = kotvSymbolRunStyle(style);
  final spans = <InlineSpan>[];
  final buf = StringBuffer();
  var kind = 0;
  void flush(int k) {
    if (buf.isEmpty) return;
    var s = buf.toString();
    buf.clear();
    if (k == 1) {
      s = s.replaceAll('\uFE0F', '').replaceAll('\uFE0E', '');
      if (s.isEmpty) return;
      spans.add(WidgetSpan(
        alignment: PlaceholderAlignment.middle,
        child: KotvSystemEmoji(text: s, logical: style.fontSize ?? 16),
      ));
      return;
    }
    spans.add(TextSpan(text: s, style: k == 2 ? symbolStyle : style));
  }

  for (final rune in text.runes) {
    final k = kotvRuneIsEmoji(rune) ? 1 : (kotvRuneIsSymbol(rune) ? 2 : 0);
    if (buf.isNotEmpty && k != kind) flush(kind);
    kind = k;
    buf.writeCharCode(rune);
  }
  flush(kind);
  return TextSpan(style: style, children: spans);
}

class KotvSystemEmoji extends StatefulWidget {
  const KotvSystemEmoji({
    super.key,
    required this.text,
    required this.logical,
  });

  final String text;
  final double logical;

  @override
  State<KotvSystemEmoji> createState() => _KotvSystemEmojiState();
}

class _KotvSystemEmojiState extends State<KotvSystemEmoji> {
  List<Uint8List>? _pngs;

  @override
  void initState() {
    super.initState();
    final cached = _emojiPngCache[widget.text];
    if (cached != null) {
      _pngs = cached;
      return;
    }
    unawaited(_load());
  }

  Future<void> _load() async {
    try {
      final font = await KotvEmojiBitmap.load();
      final pngs = font.pngsFor(widget.text);
      if (pngs.isEmpty) return;
      _emojiPngCache[widget.text] = pngs;
      if (mounted) setState(() => _pngs = pngs);
    } catch (_) {}
  }

  @override
  Widget build(BuildContext context) {
    final pngs = _pngs;
    final side = widget.logical;
    if (pngs == null || pngs.isEmpty) return SizedBox(width: side, height: side);
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        for (final bytes in pngs)
          Image.memory(
            bytes,
            height: side,
            fit: BoxFit.contain,
            gaplessPlayback: true,
            filterQuality: FilterQuality.medium,
          ),
      ],
    );
  }
}
