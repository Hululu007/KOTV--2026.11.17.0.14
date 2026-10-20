import 'dart:async';

import 'package:flutter/material.dart';

import '../theme/kotv_palette.dart';
import '../theme/kotv_theme.dart';
import '../widgets/kotv_network_image.dart';
import '../widgets/kotv_tv_text_field.dart';
import 'kotv_ui_markup.dart';

double kotvUiNum(dynamic v) => (v is num) ? v.toDouble() : 0.0;

/// 脚本长度 ≤0 表示不指定。
double? kotvUiPx(dynamic v, double scale) {
  final n = kotvUiNum(v);
  if (!(n > 0)) return null;
  return n * scale;
}

MainAxisAlignment kotvUiMainAlign(String raw) {
  switch (raw.toLowerCase().trim()) {
    case 'center':
      return MainAxisAlignment.center;
    case 'end':
    case 'right':
      return MainAxisAlignment.end;
    case 'space-between':
    case 'between':
      return MainAxisAlignment.spaceBetween;
    case 'space-around':
      return MainAxisAlignment.spaceAround;
    case 'space-evenly':
      return MainAxisAlignment.spaceEvenly;
    default:
      return MainAxisAlignment.start;
  }
}

Alignment kotvUiBoxAlign(String raw) {
  switch (kotvUiMainAlign(raw)) {
    case MainAxisAlignment.center:
      return Alignment.center;
    case MainAxisAlignment.end:
      return Alignment.centerRight;
    default:
      return Alignment.centerLeft;
  }
}

TextAlign kotvUiTextAlign(String raw) {
  switch (kotvUiMainAlign(raw)) {
    case MainAxisAlignment.center:
      return TextAlign.center;
    case MainAxisAlignment.end:
      return TextAlign.right;
    default:
      return TextAlign.left;
  }
}

/// 可点的文字。盒子只来自文档上的背景、内边距和圆角。
class KotvUiPress extends StatelessWidget {
  const KotvUiPress({
    super.key,
    required this.label,
    required this.onTap,
    required this.palette,
    required this.font,
    this.weight = FontWeight.w400,
    this.color,
    this.width,
    this.height,
    this.underline = false,
  });

  final String label;
  final VoidCallback onTap;
  final KotvPalette palette;
  final double font;
  final FontWeight weight;
  final Color? color;
  final double? width;
  final double? height;
  final bool underline;

  @override
  Widget build(BuildContext context) {
    final fg = color ?? palette.fg;
    return InkWell(
      onTap: onTap,
      child: _pressBox(
        Text(
          label,
          style: TextStyle(
            color: fg,
            fontSize: font,
            fontWeight: weight,
            decoration: underline ? TextDecoration.underline : null,
            decorationColor: fg,
          ),
        ),
        width: width,
        height: height,
      ),
    );
  }
}

Widget kotvUiActions({
  required BuildContext context,
  required List<Map<String, dynamic>> actions,
  required String align,
  required double defaultFont,
  required double defaultH,
  required double defaultW,
  required double uiScale,
  required void Function(Map<String, dynamic> action) onPressed,
}) {
  if (actions.isEmpty) return const SizedBox.shrink();
  final palette = KotvPalette.of(context);
  return Row(
    mainAxisAlignment: kotvUiMainAlign(align),
    children: [
      for (final action in actions)
        _paint(
          action,
          KotvUiPress(
            label: '${action['label'] ?? action['id'] ?? ''}',
            palette: palette,
            font: kotvUiPx(action['fontSize'], uiScale) ?? defaultFont,
            weight: kotvUiFontWeight(action),
            color: kotvUiColor(action['color']),
            width: _actionWidth(action, defaultW, uiScale),
            height: kotvUiPx(action['height'], uiScale) ?? (defaultH > 0 ? defaultH : null),
            onTap: () => onPressed(action),
          ),
          uiScale,
        ),
    ],
  );
}

double? _actionWidth(Map<String, dynamic> action, double fallback, double uiScale) {
  return kotvUiPx(action['width'], uiScale) ?? (fallback > 0 ? fallback * uiScale : null);
}

Widget _pressBox(Widget child, {double? width, double? height}) {
  if ((width == null || width <= 0) && (height == null || height <= 0)) return child;
  return SizedBox(width: width != null && width > 0 ? width : null, height: height != null && height > 0 ? height : null, child: child);
}

List<Widget> kotvUiElements(
  Map<String, dynamic> el, {
  required KotvPalette palette,
  required double docFontSize,
  required double uiScale,
  required Map<String, TextEditingController> values,
  required Map<String, bool> checks,
  required Map<String, String> radios,
  required Map<String, String> selects,
  required void Function(VoidCallback) setLocal,
  required Future<void> Function(String action, {bool dismissAfter}) fire,
  required Future<void> Function(String url) openExternal,
}) {
  final built = _raw(
    el,
    palette: palette,
    docFontSize: docFontSize,
    uiScale: uiScale,
    values: values,
    checks: checks,
    radios: radios,
    selects: selects,
    setLocal: setLocal,
    fire: fire,
    openExternal: openExternal,
  );
  if (built.isEmpty) return built;
  final inner = built.length == 1
      ? built.first
      : Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: built,
        );
  return [
    _tap(
      el,
      fire,
      openExternal,
      _paint(el, inner, uiScale),
    ),
  ];
}

Widget _tap(
  Map<String, dynamic> el,
  Future<void> Function(String action, {bool dismissAfter}) fire,
  Future<void> Function(String url) openExternal,
  Widget child,
) {
  final id = '${el['id'] ?? ''}'.trim();
  final url = '${el['url'] ?? ''}'.trim();
  final type = '${el['type'] ?? ''}'.toLowerCase().trim();
  if (type == 'button' || type == 'link' || type == 'input' || type == 'switch' || type == 'checkbox' || type == 'radio' || type == 'select' || type == 'slider' || type == 'color') {
    return child;
  }
  if (id.isEmpty && url.isEmpty) return child;
  return GestureDetector(
    behavior: HitTestBehavior.opaque,
    onTap: () async {
      if (url.isNotEmpty) await openExternal(url);
      if (id.isNotEmpty) await fire(id, dismissAfter: el['dismiss'] == true);
    },
    child: child,
  );
}

Widget _paint(Map<String, dynamic> el, Widget child, double uiScale) {
  final color = kotvUiColor(el['color']);
  final bg = kotvUiColor(el['background']);
  final border = kotvUiColor(el['border']);
  final pad = el['padding'] == null ? 0.0 : kotvUiNum(el['padding']) * uiScale;
  final margin = el['margin'] == null ? 0.0 : kotvUiNum(el['margin']) * uiScale;
  final radius = el['radius'] == null ? 0.0 : kotvUiNum(el['radius']) * uiScale;
  final align = '${el['align'] ?? ''}'.toLowerCase().trim();
  var out = child;
  if (color != null && '${el['type'] ?? ''}'.toLowerCase().trim() != 'row') {
    out = DefaultTextStyle.merge(style: TextStyle(color: color), child: out);
  }
  if (bg != null || border != null || radius > 0 || pad > 0) {
    out = DecoratedBox(
      decoration: BoxDecoration(
        color: bg,
        borderRadius: radius > 0 ? BorderRadius.circular(radius) : null,
        border: border == null ? null : Border.all(color: border),
      ),
      child: Padding(padding: EdgeInsets.all(pad), child: out),
    );
  }
  if (margin > 0) out = Padding(padding: EdgeInsets.all(margin), child: out);
  final type = '${el['type'] ?? ''}'.toLowerCase().trim();
  final hint = '${el['hint'] ?? ''}'.trim();
  final fills = type == 'button' && (hint.isNotEmpty || el['mark'] != null);
  final hug = type == 'image' || type == 'progress' || type == 'link' || (type == 'button' && !fills);
  if (hug) {
    out = Align(alignment: kotvUiBoxAlign(align), widthFactor: 1, heightFactor: 1, child: out);
  }
  return out;
}

FontWeight kotvUiFontWeight(Map<String, dynamic> el) {
  final n = kotvUiNum(el['fontWeight']);
  if (n >= 800) return FontWeight.w800;
  if (n >= 700) return FontWeight.w700;
  if (n >= 600) return FontWeight.w600;
  if (n >= 500) return FontWeight.w500;
  return FontWeight.w400;
}

String _hexText(Color color) {
  String ch(int n) => n.toRadixString(16).padLeft(2, '0');
  return '#${ch(color.red)}${ch(color.green)}${ch(color.blue)}';
}

double _fontOf(Map<String, dynamic> el, double docFont) {
  final own = kotvUiNum(el['fontSize']);
  if (own > 0) return own;
  if (docFont > 0) return docFont;
  return 14.0;
}

Widget _sized(Map<String, dynamic> el, Widget child, double uiScale) {
  final w = kotvUiPx(el['width'], uiScale);
  final h = kotvUiPx(el['height'], uiScale);
  if (w == null && h == null) return child;
  return SizedBox(width: w, height: h, child: child);
}

List<Widget> _raw(
  Map<String, dynamic> el, {
  required KotvPalette palette,
  required double docFontSize,
  required double uiScale,
  required Map<String, TextEditingController> values,
  required Map<String, bool> checks,
  required Map<String, String> radios,
  required Map<String, String> selects,
  required void Function(VoidCallback) setLocal,
  required Future<void> Function(String action, {bool dismissAfter}) fire,
  required Future<void> Function(String url) openExternal,
}) {
  final type = '${el['type'] ?? ''}'.toLowerCase().trim();
  final font = _fontOf(el, docFontSize) * uiScale;
  switch (type) {
    case 'text':
      final rawText = '${el['text'] ?? ''}';
      return [
        Text(
          rawText,
          textAlign: kotvUiTextAlign('${el['align'] ?? ''}'),
          style: TextStyle(
            color: kotvUiInk(el['color'], palette.fg),
            fontSize: font,
            fontWeight: kotvUiFontWeight(el),
          ),
        ),
      ];
    case 'image':
      final source = '${el['source'] ?? ''}'.trim();
      if (source.isEmpty) return const [];
      return [
        _sized(
          el,
          KotvNetworkImage(source, fit: BoxFit.contain),
          uiScale,
        ),
      ];
    case 'progress':
      return [
        CircularProgressIndicator(color: kotvUiInk(el['color'], palette.primary)),
      ];
    case 'slider':
      final sliderId = '${el['id'] ?? ''}'.trim();
      if (sliderId.isEmpty) return const [];
      final min = kotvUiNum(el['min']);
      var max = kotvUiNum(el['max']);
      if (max <= min) max = min + 1;
      final step = kotvUiNum(el['step']);
      final sliderCtrl = values.putIfAbsent(sliderId, () => TextEditingController(text: '${el['value'] ?? min}'));
      var current = double.tryParse(sliderCtrl.text) ?? min;
      if (current < min) current = min;
      if (current > max) current = max;
      final divisions = step > 0 ? ((max - min) / step).round().clamp(1, 1000) : null;
      return [
        Slider(
          value: current,
          min: min,
          max: max,
          divisions: divisions,
          label: sliderCtrl.text,
          activeColor: kotvUiInk(el['color'], palette.primary),
          onChanged: (v) {
            final stepped = step > 0 ? (v / step).round() * step : v;
            final clamped = stepped < min ? min : (stepped > max ? max : stepped);
            sliderCtrl.text = clamped == clamped.roundToDouble() ? '${clamped.round()}' : clamped.toStringAsFixed(2);
            setLocal(() {});
          },
        ),
      ];
    case 'color':
      final colorId = '${el['id'] ?? ''}'.trim();
      if (colorId.isEmpty) return const [];
      final colorCtrl = values.putIfAbsent(colorId, () => TextEditingController(text: '${el['value'] ?? '#ffffff'}'));
      final picked = kotvUiInk(colorCtrl.text, Colors.white);
      final hsv = HSVColor.fromColor(picked);
      void write(HSVColor next) {
        colorCtrl.text = _hexText(next.toColor());
        setLocal(() {});
      }
      final gap = kotvUiPx(el['spacing'], uiScale) ?? 0;
      final swatch = kotvUiPx(el['height'], uiScale) ?? font;
      final edge = kotvUiColor(el['border']);
      Widget band(String label, double value, double max, ValueChanged<double> onChanged) {
        return Row(
          children: [
            Text(label, style: TextStyle(color: kotvUiInk(el['hintColor'], palette.muted), fontSize: font)),
            if (gap > 0) SizedBox(width: gap),
            Expanded(
              child: Slider(
                value: value.clamp(0, max).toDouble(),
                min: 0,
                max: max,
                activeColor: picked,
                onChanged: onChanged,
              ),
            ),
          ],
        );
      }
      return [
        Row(
          children: [
            Container(
              width: swatch,
              height: swatch,
              decoration: BoxDecoration(
                color: picked,
                shape: BoxShape.circle,
                border: edge == null ? null : Border.all(color: edge),
              ),
            ),
            if (gap > 0) SizedBox(width: gap),
            Expanded(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  band('色相', hsv.hue, 360, (h) => write(hsv.withHue(h))),
                  band('饱和', hsv.saturation, 1, (s) => write(hsv.withSaturation(s))),
                  band('明度', hsv.value, 1, (v) => write(hsv.withValue(v))),
                ],
              ),
            ),
          ],
        ),
      ];
    case 'input':
      final id = '${el['id'] ?? ''}';
      if (id.isEmpty) return const [];
      final ctrl = values.putIfAbsent(id, () => TextEditingController(text: '${el['value'] ?? ''}'));
      final secret = el['password'] == true;
      if (secret) checks.putIfAbsent('obscure:$id', () => true);
      final multi = el['multiline'] == true;
      final field = KotvTvTextField(
        controller: ctrl,
        obscureText: secret && (checks['obscure:$id'] ?? true),
        maxLines: multi ? null : 1,
        minLines: multi ? 1 : null,
        style: TextStyle(color: kotvUiInk(el['color'], palette.fg), fontSize: font),
        cursorColor: kotvUiInk(el['color'], palette.primary),
        decoration: InputDecoration(
          hintText: '${el['placeholder'] ?? ''}',
          hintStyle: TextStyle(color: kotvUiInk(el['hintColor'], palette.muted), fontSize: font),
          filled: false,
          border: InputBorder.none,
          enabledBorder: InputBorder.none,
          focusedBorder: InputBorder.none,
          isCollapsed: true,
          contentPadding: EdgeInsets.zero,
        ),
      );
      return [_sized(el, field, uiScale)];
    case 'switch':
      final id = '${el['id'] ?? ''}';
      if (id.isEmpty) return const [];
      checks.putIfAbsent(id, () => el['checked'] == true);
      final control = Switch(
        value: checks[id] ?? false,
        thumbColor: MaterialStateProperty.resolveWith((states) {
          if (states.contains(MaterialState.selected)) return Colors.white;
          return null;
        }),
        trackColor: MaterialStateProperty.resolveWith((states) {
          if (states.contains(MaterialState.selected)) return kotvUiInk(el['color'], palette.primary);
          return null;
        }),
        onChanged: (v) => setLocal(() => checks[id] = v),
      );
      final caption = '${el['text'] ?? ''}';
      if (caption.isEmpty) return [control];
      return [
        Row(
          children: [
            Expanded(child: Text(caption, style: TextStyle(color: palette.fg, fontSize: font))),
            control,
          ],
        ),
      ];
    case 'checkbox':
      final id = '${el['id'] ?? ''}';
      if (id.isEmpty) return const [];
      checks.putIfAbsent(id, () => el['checked'] == true);
      return [
        TvFocus(
          onPressed: () => setLocal(() => checks[id] = !(checks[id] ?? false)),
          borderRadius: 0,
          scrollIntoView: true,
          child: ExcludeFocus(
            child: Row(
              children: [
                Checkbox(
                  value: checks[id] ?? false,
                  onChanged: (v) => setLocal(() => checks[id] = v ?? false),
                  activeColor: kotvUiInk(el['color'], palette.primary),
                  materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
                ),
                Expanded(
                  child: Text('${el['text'] ?? ''}', style: TextStyle(color: kotvUiInk(el['color'], palette.fg), fontSize: font)),
                ),
              ],
            ),
          ),
        ),
      ];
    case 'radio':
      final id = '${el['id'] ?? ''}';
      if (id.isEmpty) return const [];
      final options = (el['options'] as List?) ?? const [];
      radios.putIfAbsent(id, () => '${el['value'] ?? ''}');
      return [
        for (final o in options)
          if (o is Map)
            TvFocus(
              onPressed: () => setLocal(() => radios[id] = '${o['id'] ?? ''}'),
              borderRadius: 0,
              scrollIntoView: true,
              child: ExcludeFocus(
                child: Row(
                  children: [
                    Radio<String>(
                      value: '${o['id'] ?? ''}',
                      groupValue: radios[id],
                      onChanged: (v) => setLocal(() => radios[id] = v ?? ''),
                      activeColor: kotvUiInk(el['color'], palette.primary),
                      materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
                    ),
                    Expanded(
                      child: Text('${o['label'] ?? o['id'] ?? ''}', style: TextStyle(color: kotvUiInk(el['color'], palette.fg), fontSize: font)),
                    ),
                  ],
                ),
              ),
            ),
      ];
    case 'select':
      final id = '${el['id'] ?? ''}';
      if (id.isEmpty) return const [];
      final options = (el['options'] as List?) ?? const [];
      selects.putIfAbsent(id, () => '${el['value'] ?? (options.isNotEmpty && options.first is Map ? options.first['id'] : '')}');
      final dropdown = DropdownButtonFormField<String>(
        value: selects[id]?.isEmpty == true ? null : selects[id],
        focusColor: kotvUiColor(el['color']),
        dropdownColor: kotvUiInk(el['background'], palette.dialogBg),
        style: TextStyle(color: kotvUiInk(el['color'], palette.fg), fontSize: font),
        items: [
          for (final o in options)
            if (o is Map)
              DropdownMenuItem(
                value: '${o['id']}',
                child: Text('${o['label'] ?? o['id']}', style: TextStyle(color: kotvUiInk(el['color'], palette.fg), fontSize: font)),
              ),
        ],
        onChanged: (v) => setLocal(() => selects[id] = v ?? ''),
      );
      return [_sized(el, dropdown, uiScale)];
    case 'button':
      final btnUrl = '${el['url'] ?? ''}'.trim();
      final toggleId = '${el['toggle'] ?? ''}'.trim();
      Future<void> onButton() async {
        if (toggleId.isNotEmpty) {
          setLocal(() {
            final key = 'obscure:$toggleId';
            checks[key] = !(checks[key] ?? true);
          });
          return;
        }
        if (btnUrl.isNotEmpty) await openExternal(btnUrl);
        await fire('${el['id'] ?? 'button'}', dismissAfter: el['dismiss'] == true);
      }
      final hint = '${el['hint'] ?? ''}'.trim();
      final mark = kotvUiColor(el['mark']);
      if (hint.isNotEmpty || mark != null) {
        final gap = kotvUiPx(el['spacing'], uiScale) ?? 0;
        return [
          InkWell(
            onTap: () => unawaited(onButton()),
            child: Row(
              children: [
                if (mark != null) ...[
                  Container(
                    width: font,
                    height: font,
                    decoration: BoxDecoration(color: mark, shape: BoxShape.circle),
                  ),
                  if (gap > 0) SizedBox(width: gap),
                ],
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        '${el['text'] ?? ''}',
                        style: TextStyle(
                          color: kotvUiInk(el['color'], palette.fg),
                          fontSize: font,
                          fontWeight: kotvUiFontWeight(el),
                        ),
                      ),
                      if (hint.isNotEmpty)
                        Text(
                          hint,
                          style: TextStyle(
                            color: kotvUiInk(el['hintColor'], kotvUiInk(el['color'], palette.fg)),
                            fontSize: font,
                          ),
                        ),
                    ],
                  ),
                ),
              ],
            ),
          ),
        ];
      }
      return [
        KotvUiPress(
          label: '${el['text'] ?? ''}',
          palette: palette,
          font: font,
          weight: kotvUiFontWeight(el),
          color: kotvUiColor(el['color']),
          width: kotvUiPx(el['width'], uiScale),
          height: kotvUiPx(el['height'], uiScale),
          onTap: () => unawaited(onButton()),
        ),
      ];
    case 'link':
      final linkUrl = '${el['url'] ?? ''}'.trim();
      if (linkUrl.isEmpty) return const [];
      final labelText = '${el['text'] ?? ''}'.trim().isEmpty ? linkUrl : '${el['text']}'.trim();
      final look = '${el['style'] ?? ''}'.toLowerCase().trim();
      return [
        KotvUiPress(
          label: labelText,
          underline: look == 'underline',
          palette: palette,
          font: font,
          weight: kotvUiFontWeight(el),
          color: kotvUiColor(el['color']),
          width: kotvUiPx(el['width'], uiScale),
          height: kotvUiPx(el['height'], uiScale),
          onTap: () => unawaited(openExternal(linkUrl)),
        ),
      ];
    case 'separator':
      final thickness = kotvUiPx(el['height'], uiScale) ?? uiScale;
      return [
        LayoutBuilder(
          builder: (context, constraints) {
            final span = constraints.maxWidth.isFinite ? constraints.maxWidth : 0.0;
            return SizedBox(width: span, height: thickness, child: ColoredBox(color: kotvUiInk(el['color'], palette.outline)));
          },
        ),
      ];
    case 'html':
      final markup = '${el['html'] ?? el['text'] ?? ''}'.trim();
      if (markup.isEmpty) return const [];
      return [
        KotvUiMarkup(
          html: markup,
          color: kotvUiInk(el['color'], palette.fg),
          fontSize: font,
          scale: uiScale,
          onAction: (id) => unawaited(fire(id)),
          onLink: (url) => unawaited(openExternal(url)),
        ),
      ];
    case 'spacer':
    case 'space':
      final sp = kotvUiNum(el['height']) * uiScale;
      if (sp <= 0) return const [];
      return [SizedBox(height: sp)];
    case 'row':
    case 'column':
    case 'group':
      return [_flex(
        el,
        type: type,
        palette: palette,
        docFontSize: docFontSize,
        uiScale: uiScale,
        values: values,
        checks: checks,
        radios: radios,
        selects: selects,
        setLocal: setLocal,
        fire: fire,
        openExternal: openExternal,
        font: font,
      )];
    default:
      return const [];
  }
}

Widget _flex(
  Map<String, dynamic> el, {
  required String type,
  required KotvPalette palette,
  required double docFontSize,
  required double uiScale,
  required Map<String, TextEditingController> values,
  required Map<String, bool> checks,
  required Map<String, String> radios,
  required Map<String, String> selects,
  required void Function(VoidCallback) setLocal,
  required Future<void> Function(String action, {bool dismissAfter}) fire,
  required Future<void> Function(String url) openExternal,
  required double font,
}) {
  final horizontal = type == 'row';
  final spacing = kotvUiPx(el['spacing'], uiScale) ?? 0;
  final align = kotvUiMainAlign('${el['align'] ?? ''}');
  final boxH = kotvUiNum(el['height']) * uiScale;
  final flex = horizontal || boxH > 0;
  final slots = <Widget>[
    if (type == 'group' && '${el['text'] ?? ''}'.isNotEmpty)
      Text(
        '${el['text']}',
        textAlign: kotvUiTextAlign('${el['align'] ?? ''}'),
        style: TextStyle(color: kotvUiInk(el['color'], palette.fg), fontWeight: kotvUiFontWeight(el), fontSize: font),
      ),
  ];
  final children = (el['children'] as List?) ?? const [];
  for (final raw in children) {
    if (raw is! Map) continue;
    final child = Map<String, dynamic>.from(raw);
    final built = kotvUiElements(
      child,
      palette: palette,
      docFontSize: docFontSize,
      uiScale: uiScale,
      values: values,
      checks: checks,
      radios: radios,
      selects: selects,
      setLocal: setLocal,
      fire: fire,
      openExternal: openExternal,
    );
    final box = Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.start,
      children: built,
    );
    final weight = kotvUiNum(child['weight']);
    if (flex && weight > 0) {
      slots.add(Expanded(flex: weight.round().clamp(1, 100), child: box));
    } else {
      slots.add(box);
    }
  }
  final spaced = <Widget>[];
  for (var i = 0; i < slots.length; i++) {
    if (i > 0 && spacing > 0) {
      spaced.add(horizontal ? SizedBox(width: spacing) : SizedBox(height: spacing));
    }
    spaced.add(slots[i]);
  }
  if (horizontal) {
    return Row(
      mainAxisAlignment: align,
      crossAxisAlignment: CrossAxisAlignment.center,
      children: spaced,
    );
  }
  final column = Column(
    mainAxisSize: boxH > 0 ? MainAxisSize.max : MainAxisSize.min,
    crossAxisAlignment: CrossAxisAlignment.stretch,
    children: spaced,
  );
  if (boxH <= 0) return column;
  return SizedBox(height: boxH, child: column);
}
