import 'package:flutter/material.dart';
import 'package:flutter_html/flutter_html.dart';
import 'package:html/dom.dart' as dom;
import 'package:html/parser.dart' as html_parser;

/// 解析 #RGB、#RRGGBB、#AARRGGBB。空或无法识别时返回 null。
Color? kotvUiColor(Object? raw) {
  var s = '${raw ?? ''}'.trim();
  if (s.isEmpty || s == 'null') return null;
  if (s.startsWith('#')) s = s.substring(1);
  if (s.length == 3) {
    s = s.split('').map((c) => '$c$c').join();
  }
  if (s.length == 6) s = 'FF$s';
  if (s.length != 8) return null;
  final v = int.tryParse(s, radix: 16);
  if (v == null) return null;
  return Color(v);
}

/// 爬虫给出的 HTML。不执行脚本，不嵌网页。
/// `href="action:id"` 回报该动作，其它链接交给 [onLink]。
/// `position:absolute` / `fixed` 按 left/top/z-index 叠在正文上。
class KotvUiMarkup extends StatelessWidget {
  const KotvUiMarkup({
    super.key,
    required this.html,
    required this.color,
    required this.fontSize,
    required this.onAction,
    required this.onLink,
    this.scale = 1,
  });

  final String html;
  final Color color;
  final double fontSize;
  final void Function(String action) onAction;
  final void Function(String url) onLink;
  final double scale;

  static const _skip = {
    'script',
    'iframe',
    'object',
    'embed',
    'applet',
    'form',
    'link',
    'meta',
  };

  @override
  Widget build(BuildContext context) {
    final flow = _HtmlFlow(html);
    final body = Html(
      data: flow.flowHtml,
      shrinkWrap: true,
      doNotRenderTheseTags: _skip,
      style: {
        'body': Style(
          margin: Margins.zero,
          padding: HtmlPaddings.zero,
          color: color,
          fontSize: FontSize(fontSize),
        ),
        'html': Style(
          margin: Margins.zero,
          padding: HtmlPaddings.zero,
        ),
      },
      onLinkTap: (url, attributes, element) => _tap(url, attributes),
    );
    if (flow.placed.isEmpty) return body;
    final stack = Stack(
      clipBehavior: Clip.none,
      children: [
        if (flow.flowHtml.trim().isNotEmpty) body,
        for (final box in flow.placed)
          Positioned(
            left: box.left == null ? null : box.left! * scale,
            top: box.top == null ? null : box.top! * scale,
            right: box.right == null ? null : box.right! * scale,
            bottom: box.bottom == null ? null : box.bottom! * scale,
            width: box.width == null ? null : box.width! * scale,
            height: box.height == null ? null : box.height! * scale,
            child: KotvUiMarkup(
              html: box.html,
              color: color,
              fontSize: fontSize,
              scale: scale,
              onAction: onAction,
              onLink: onLink,
            ),
          ),
      ],
    );
    if (flow.flowHtml.trim().isNotEmpty) return stack;
    var right = 0.0;
    var bottom = 0.0;
    for (final box in flow.placed) {
      final edgeX = (box.left ?? 0) + (box.width ?? 0);
      final edgeY = (box.top ?? 0) + (box.height ?? 0);
      if (edgeX > right) right = edgeX;
      if (edgeY > bottom) bottom = edgeY;
    }
    return SizedBox(
      width: right > 0 ? right * scale : null,
      height: bottom > 0 ? bottom * scale : null,
      child: stack,
    );
  }

  void _tap(String? url, Map<String, String> attributes) {
    final href = (url ?? attributes['href'] ?? '').trim();
    if (href.isEmpty) return;
    final action = _actionOf(href);
    if (action != null) {
      onAction(action);
      return;
    }
    final lower = href.toLowerCase();
    if (lower.startsWith('javascript:') || lower.startsWith('data:')) return;
    onLink(href);
  }

  static String? _actionOf(String href) {
    const prefix = 'action:';
    if (href.length > prefix.length && href.toLowerCase().startsWith(prefix)) {
      final id = href.substring(prefix.length).trim();
      if (id.isEmpty) return null;
      return id;
    }
    return null;
  }
}

class _PlacedBox {
  _PlacedBox({
    required this.html,
    required this.z,
    this.left,
    this.top,
    this.right,
    this.bottom,
    this.width,
    this.height,
  });

  final String html;
  final int z;
  final double? left;
  final double? top;
  final double? right;
  final double? bottom;
  final double? width;
  final double? height;
}

class _HtmlFlow {
  _HtmlFlow(String html) : flowHtml = html, placed = const [] {
    dom.Document document;
    try {
      document = html_parser.parse(html);
    } catch (_) {
      return;
    }
    final body = document.body;
    if (body == null) return;
    final found = _hoist(body);
    if (found.isEmpty) return;
    found.sort((a, b) => a.z.compareTo(b.z));
    flowHtml = body.innerHtml;
    placed = found;
  }

  String flowHtml;
  List<_PlacedBox> placed;

  static List<_PlacedBox> _hoist(dom.Element parent) {
    final found = <_PlacedBox>[];
    for (final node in List<dom.Node>.from(parent.nodes)) {
      if (node is! dom.Element) continue;
      final style = _css(node.attributes['style']);
      final position = style['position'] ?? '';
      if (position == 'absolute' || position == 'fixed') {
        final copy = node.clone(true);
        final z = int.tryParse(style['z-index'] ?? '') ?? 0;
        final left = _px(style['left']);
        final top = _px(style['top']);
        final right = _px(style['right']);
        final bottom = _px(style['bottom']);
        final width = _px(style['width']);
        final height = _px(style['height']);
        for (final key in ['position', 'left', 'top', 'right', 'bottom', 'z-index']) {
          style.remove(key);
        }
        if (style.isEmpty) {
          copy.attributes.remove('style');
        } else {
          copy.attributes['style'] = style.entries.map((e) => '${e.key}:${e.value}').join(';');
        }
        found.add(_PlacedBox(
          html: copy.outerHtml,
          z: z,
          left: left,
          top: top,
          right: right,
          bottom: bottom,
          width: width,
          height: height,
        ));
        node.remove();
      } else {
        found.addAll(_hoist(node));
      }
    }
    return found;
  }

  static Map<String, String> _css(String? raw) {
    final out = <String, String>{};
    if (raw == null || raw.isEmpty) return out;
    for (final part in raw.split(';')) {
      final i = part.indexOf(':');
      if (i <= 0) continue;
      out[part.substring(0, i).trim().toLowerCase()] = part.substring(i + 1).trim();
    }
    return out;
  }

  static double? _px(String? raw) {
    if (raw == null || raw.isEmpty || raw == 'auto') return null;
    final match = RegExp(r'-?\d+(\.\d+)?').firstMatch(raw);
    if (match == null) return null;
    return double.tryParse(match.group(0)!);
  }
}
