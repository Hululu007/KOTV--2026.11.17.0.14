import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../remote/download_settings.dart';
import '../remote/local_download.dart';
import '../theme/kotv_palette.dart';
import '../theme/layout_scale.dart';
import '../widgets/chrome.dart';
import '../widgets/h_scroll.dart';
import '../widgets/kotv_network_image.dart';
import 'shell.dart';

class DownloadScreen extends ConsumerStatefulWidget {
  const DownloadScreen({super.key});

  @override
  ConsumerState<DownloadScreen> createState() => _DownloadScreenState();
}

class _DownloadScreenState extends ConsumerState<DownloadScreen> {
  List<Map<String, dynamic>> _tasks = [];
  final _selected = <String>{};
  String _filter = 'all';
  bool _loading = true;
  bool _selectMode = false;
  String? _status;
  Timer? _poll;

  @override
  void initState() {
    super.initState();
    _reload();
    _poll =
        Timer.periodic(const Duration(seconds: 1), (_) => _reload(silent: true));
  }

  @override
  void dispose() {
    _poll?.cancel();
    super.dispose();
  }

  List<Map<String, dynamic>> get _visible => _tasks
      .where((t) =>
          DownloadSettings.matchFilter('${t['status'] ?? ''}', _filter))
      .toList();

  Future<void> _reload({bool silent = false}) async {
    if (!silent && mounted) setState(() => _loading = true);
    try {
      final list = await LocalDownload.list();
      if (!mounted) return;
      final ids = list.map((e) => '${e['id']}').toSet();
      _selected.removeWhere((id) => !ids.contains(id));
      setState(() {
        _tasks = list;
        _loading = false;
        if (!silent) _status = null;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _loading = false;
        if (!silent) _status = '$e';
      });
    }
  }

  String _label(Map<String, dynamic> t) {
    final title = '${t['title'] ?? ''}'.trim();
    final ep = '${t['epName'] ?? ''}'.trim();
    final flag = '${t['flag'] ?? ''}'.trim();
    final parts = <String>[
      if (title.isNotEmpty) title,
      if (flag.isNotEmpty) flag,
      if (ep.isNotEmpty) ep,
    ];
    return parts.isEmpty ? '${t['id'] ?? ''}' : parts.join(' · ');
  }

  String _statusText(Map<String, dynamic> t) {
    final st = '${t['status'] ?? ''}';
    final msg = '${t['message'] ?? ''}'.trim();
    final err = '${t['error'] ?? ''}'.trim();
    switch (st) {
      case 'queued':
        return msg.isEmpty ? '等待中' : msg;
      case 'running':
        final p =
            (t['progress'] is num) ? (t['progress'] as num).toDouble() : 0.0;
        final pct = (p * 100).clamp(0, 100).toStringAsFixed(0);
        return msg.isEmpty ? '下载中 $pct%' : '$msg · $pct%';
      case 'paused':
        return msg.isEmpty ? '已暂停' : msg;
      case 'done':
        return '已完成';
      case 'cancelled':
        return '已取消';
      case 'error':
        return err.isNotEmpty ? err : (msg.isEmpty ? '失败' : msg);
      default:
        return st;
    }
  }

  Future<void> _actSelected(
      Future<void> Function(String id) fn, String doneMsg) async {
    final ids = _selected.toList();
    if (ids.isEmpty) {
      setState(() => _status = '请先勾选任务');
      return;
    }
    for (final id in ids) {
      try {
        await fn(id);
      } catch (_) {}
    }
    if (!mounted) return;
    setState(() {
      _status = doneMsg;
      _selected.clear();
      _selectMode = false;
    });
    await _reload();
  }

  Widget _poster(Map<String, dynamic> t, KotvPalette p) {
    final pic = '${t['pic'] ?? ''}'.trim();
    return ClipRRect(
      borderRadius: BorderRadius.circular(8),
      child: SizedBox(
        width: 56,
        height: 78,
        child: pic.isEmpty
            ? ColoredBox(
                color: p.posterPh,
                child: Icon(Icons.movie_outlined, color: p.muted, size: 28),
              )
            : KotvNetworkImage(
                pic,
                fit: BoxFit.cover,
                errorBuilder: (_, __, ___) => ColoredBox(
                  color: p.posterPh,
                  child: Icon(Icons.broken_image_outlined,
                      color: p.muted, size: 24),
                ),
              ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final p = KotvPalette.of(context);
    final compact = KotvLayout.isCompact(context);
    final s = LayoutScale.of(context);
    final visible = _visible;
    final filterIdx =
        DownloadSettings.filterOptions.indexWhere((e) => e.$2 == _filter);
    return Column(
      children: [
        LibraryTopBar(
          title: '我的下载',
          onBack: () => kotvPageBack(ref),
          onSearch: () => goKotvPage(ref, KotvPage.search),
          onProfile: () => goKotvPage(ref, KotvPage.profile),
          onNews: () {},
          trailing: AppPill(
            label: _selectMode ? '完成' : '多选',
            height: compact ? 34 : 36,
            selected: _selectMode,
            onTap: () => setState(() {
              _selectMode = !_selectMode;
              if (!_selectMode) _selected.clear();
            }),
          ),
        ),
        Padding(
          padding: EdgeInsets.fromLTRB(12 * s, 8, 12 * s, 0),
          child: SizedBox(
            height: 36,
            child: HScrollList(
              scrollToIndex: filterIdx >= 0 ? filterIdx : 0,
              itemCount: DownloadSettings.filterOptions.length,
              separatorBuilder: (_, __) => const SizedBox(width: 8),
              itemBuilder: (_, i) {
                final e = DownloadSettings.filterOptions[i];
                return AppPill(
                  label: e.$1,
                  height: 34,
                  selected: _filter == e.$2,
                  onTap: () => setState(() => _filter = e.$2),
                );
              },
            ),
          ),
        ),
        Padding(
          padding: EdgeInsets.fromLTRB(12 * s, 8, 12 * s, 0),
          child: SingleChildScrollView(
            scrollDirection: Axis.horizontal,
            child: Row(
              children: [
                AppPill(
                  label: '全选',
                  height: 32,
                  onTap: () => setState(() {
                    _selectMode = true;
                    _selected
                      ..clear()
                      ..addAll(visible.map((e) => '${e['id']}'));
                  }),
                ),
                const SizedBox(width: 6),
                AppPill(
                  label: '开始',
                  height: 32,
                  onTap: () async {
                    if (_selectMode && _selected.isNotEmpty) {
                      await _actSelected(LocalDownload.resume, '已开始所选任务');
                    } else {
                      await LocalDownload.resumeAll();
                      if (mounted) setState(() => _status = '已开始全部可恢复任务');
                      await _reload();
                    }
                  },
                ),
                const SizedBox(width: 6),
                AppPill(
                  label: '暂停',
                  height: 32,
                  onTap: () async {
                    if (_selectMode && _selected.isNotEmpty) {
                      await _actSelected(LocalDownload.pause, '已暂停所选任务');
                    } else {
                      await LocalDownload.pauseAll();
                      if (mounted) setState(() => _status = '已暂停全部');
                      await _reload();
                    }
                  },
                ),
                const SizedBox(width: 6),
                AppPill(
                  label: '取消',
                  height: 32,
                  onTap: () async {
                    if (_selectMode && _selected.isNotEmpty) {
                      await _actSelected(LocalDownload.cancel, '已取消所选任务');
                    } else {
                      setState(() => _status = '请先多选任务再取消');
                    }
                  },
                ),
                const SizedBox(width: 6),
                AppPill(
                  label: '删除',
                  height: 32,
                  onTap: () async {
                    if (_selectMode && _selected.isNotEmpty) {
                      await _actSelected(
                          (id) => LocalDownload.remove(id), '已删除所选任务');
                    } else {
                      final n = await LocalDownload.clearFinished();
                      if (!mounted) return;
                      setState(() =>
                          _status = n > 0 ? '已清理 $n 条完成/失败' : '没有可清理项');
                      await _reload();
                    }
                  },
                ),
              ],
            ),
          ),
        ),
        if (_status != null && _status!.isNotEmpty)
          Padding(
            padding: EdgeInsets.fromLTRB(16 * s, 8, 16 * s, 0),
            child: Align(
              alignment: Alignment.centerLeft,
              child: Text(_status!, style: TextStyle(color: p.muted, fontSize: 13)),
            ),
          ),
        Expanded(
          child: _loading && _tasks.isEmpty
              ? const Center(child: CircularProgressIndicator())
              : visible.isEmpty
                  ? Center(
                      child: Text('暂无下载任务', style: TextStyle(color: p.muted)),
                    )
                  : ListView.separated(
                      padding: EdgeInsets.fromLTRB(16 * s, 12, 16 * s, 24),
                      itemCount: visible.length,
                      separatorBuilder: (_, __) => SizedBox(height: 8 * s),
                      itemBuilder: (_, i) {
                        final t = visible[i];
                        final id = '${t['id']}';
                        final st = '${t['status'] ?? ''}';
                        final prog = (t['progress'] is num)
                            ? (t['progress'] as num).toDouble().clamp(0.0, 1.0)
                            : 0.0;
                        final on = _selected.contains(id);
                        return DecoratedBox(
                          decoration: BoxDecoration(
                            color: p.pillBg,
                            borderRadius: BorderRadius.circular(12),
                            border: Border.all(
                              color: on ? p.primary : p.pillBorder,
                            ),
                          ),
                          child: InkWell(
                            borderRadius: BorderRadius.circular(12),
                            onTap: _selectMode
                                ? () => setState(() {
                                      if (on) {
                                        _selected.remove(id);
                                      } else {
                                        _selected.add(id);
                                      }
                                    })
                                : null,
                            onLongPress: () => setState(() {
                              _selectMode = true;
                              _selected.add(id);
                            }),
                            child: Padding(
                              padding: const EdgeInsets.fromLTRB(10, 10, 8, 10),
                              child: Row(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  if (_selectMode) ...[
                                    Padding(
                                      padding: const EdgeInsets.only(
                                          top: 24, right: 6),
                                      child: Icon(
                                        on
                                            ? Icons.check_circle
                                            : Icons.circle_outlined,
                                        color: on ? p.primary : p.muted,
                                        size: 22,
                                      ),
                                    ),
                                  ],
                                  _poster(t, p),
                                  const SizedBox(width: 10),
                                  Expanded(
                                    child: Column(
                                      crossAxisAlignment:
                                          CrossAxisAlignment.stretch,
                                      children: [
                                        Text(
                                          _label(t),
                                          maxLines: 2,
                                          overflow: TextOverflow.ellipsis,
                                          style: TextStyle(
                                            color: p.fg,
                                            fontSize: 14,
                                            fontWeight: FontWeight.w600,
                                          ),
                                        ),
                                        const SizedBox(height: 4),
                                        Text(
                                          _statusText(t),
                                          style: TextStyle(
                                              color: p.muted, fontSize: 12),
                                        ),
                                        if (st == 'running' ||
                                            (st == 'paused' && prog > 0)) ...[
                                          const SizedBox(height: 8),
                                          ClipRRect(
                                            borderRadius:
                                                BorderRadius.circular(4),
                                            child: LinearProgressIndicator(
                                              value: prog > 0 ? prog : null,
                                              minHeight: 4,
                                              backgroundColor:
                                                  p.fg.withValues(alpha: 0.08),
                                              color: p.primary,
                                            ),
                                          ),
                                        ],
                                        if (!_selectMode) ...[
                                          const SizedBox(height: 8),
                                          Wrap(
                                            spacing: 6,
                                            runSpacing: 6,
                                            children: [
                                              if (st == 'running' ||
                                                  st == 'queued')
                                                AppPill(
                                                  label: '暂停',
                                                  height: 30,
                                                  width: 56,
                                                  onTap: () async {
                                                    await LocalDownload.pause(
                                                        id);
                                                    await _reload();
                                                  },
                                                ),
                                              if (st == 'paused' ||
                                                  st == 'error' ||
                                                  st == 'cancelled')
                                                AppPill(
                                                  label: '开始',
                                                  height: 30,
                                                  width: 56,
                                                  onTap: () async {
                                                    await LocalDownload.resume(
                                                        id);
                                                    await _reload();
                                                  },
                                                ),
                                              if (st == 'running' ||
                                                  st == 'queued' ||
                                                  st == 'paused')
                                                AppPill(
                                                  label: '取消',
                                                  height: 30,
                                                  width: 56,
                                                  onTap: () async {
                                                    await LocalDownload.cancel(
                                                        id);
                                                    await _reload();
                                                  },
                                                ),
                                              AppPill(
                                                label: '删除',
                                                height: 30,
                                                width: 56,
                                                onTap: () async {
                                                  await LocalDownload.remove(id);
                                                  await _reload();
                                                },
                                              ),
                                            ],
                                          ),
                                        ],
                                      ],
                                    ),
                                  ),
                                ],
                              ),
                            ),
                          ),
                        );
                      },
                    ),
        ),
      ],
    );
  }
}
