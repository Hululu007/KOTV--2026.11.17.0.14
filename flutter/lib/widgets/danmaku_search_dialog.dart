import 'package:flutter/material.dart';

import '../api/kotv_api.dart';
import '../player/danmaku_layer.dart';
import '../theme/kotv_palette.dart';
import 'chrome.dart';
import 'kotv_tv_text_field.dart';

/// 搜索弹幕：片名、集名可改，经引擎调弹幕搜索接口，点一条结果返回。
Future<DanmakuSource?> showKotvDanmakuSearchDialog(
  BuildContext context, {
  required KotvApi api,
  required String name,
  required String episode,
}) {
  return showDialog<DanmakuSource>(
    context: context,
    builder: (ctx) => _DanmakuSearchDialog(api: api, name: name, episode: episode),
  );
}

class _DanmakuSearchDialog extends StatefulWidget {
  const _DanmakuSearchDialog({required this.api, required this.name, required this.episode});

  final KotvApi api;
  final String name;
  final String episode;

  @override
  State<_DanmakuSearchDialog> createState() => _DanmakuSearchDialogState();
}

class _DanmakuSearchDialogState extends State<_DanmakuSearchDialog> {
  late final TextEditingController _name = TextEditingController(text: widget.name);
  late final TextEditingController _episode = TextEditingController(text: widget.episode);
  List<DanmakuSource> _results = const [];
  bool _busy = false;
  String _status = '';

  @override
  void dispose() {
    _name.dispose();
    _episode.dispose();
    super.dispose();
  }

  Future<void> _search() async {
    final name = _name.text.trim();
    if (name.isEmpty || _busy) return;
    setState(() {
      _busy = true;
      _status = '搜索中…';
    });
    try {
      final res = await widget.api.danmakuSearch(name: name, episode: _episode.text.trim());
      final found = kotvDanmakuSourcesFrom(res['sources']);
      if (!mounted) return;
      setState(() {
        _results = found;
        _status = found.isEmpty ? '没有找到弹幕' : '';
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _results = const [];
        _status = '搜索失败：$e';
      });
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final p = KotvPalette.of(context);
    InputDecoration deco(String hint) => InputDecoration(
          hintText: hint,
          hintStyle: TextStyle(color: p.muted),
          filled: true,
          fillColor: p.input,
          isDense: true,
          border: OutlineInputBorder(borderRadius: BorderRadius.circular(10), borderSide: BorderSide.none),
        );
    return AlertDialog(
      backgroundColor: p.dialogBg,
      title: Text('搜索弹幕', style: TextStyle(color: p.fg, fontWeight: FontWeight.w700)),
      content: SizedBox(
        width: 420,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            KotvTvTextField(
              controller: _name,
              autofocus: true,
              style: TextStyle(color: p.fg),
              decoration: deco('片名'),
              onSubmitted: (_) => _search(),
            ),
            const SizedBox(height: 8),
            KotvTvTextField(
              controller: _episode,
              style: TextStyle(color: p.fg),
              decoration: deco('集名（可空）'),
              onSubmitted: (_) => _search(),
            ),
            if (_status.isNotEmpty)
              Padding(
                padding: const EdgeInsets.only(top: 10),
                child: Text(_status, style: TextStyle(color: p.muted)),
              ),
            if (_results.isNotEmpty)
              Flexible(
                child: ListView(
                  shrinkWrap: true,
                  children: [
                    for (final s in _results)
                      ListTile(
                        dense: true,
                        leading: Icon(Icons.subtitles_outlined, color: p.muted),
                        title: Text(s.name, style: TextStyle(color: p.fg)),
                        onTap: () => Navigator.pop(context, s),
                      ),
                  ],
                ),
              ),
          ],
        ),
      ),
      actions: [
        KotvFocusAction(label: '关闭', color: p.muted, onPressed: () => Navigator.pop(context)),
        KotvFocusAction(label: '搜索', filled: true, onPressed: _search),
      ],
    );
  }
}
