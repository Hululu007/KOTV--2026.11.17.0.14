import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../nav/kotv_page.dart';
import '../remote/local_download.dart';
import '../theme/kotv_palette.dart';
import '../theme/layout_scale.dart';
import '../widgets/chrome.dart';
import 'shell.dart';

class DownloadScreen extends ConsumerStatefulWidget {
  const DownloadScreen({super.key});

  @override
  ConsumerState<DownloadScreen> createState() => _DownloadScreenState();
}

class _DownloadScreenState extends ConsumerState<DownloadScreen> {
  List<Map<String, dynamic>> _tasks = [];
  bool _loading = true;
  String? _status;
  Timer? _poll;

  @override
  void initState() {
    super.initState();
    _reload();
    _poll = Timer.periodic(const Duration(seconds: 1), (_) => _reload(silent: true));
  }

  @override
  void dispose() {
    _poll?.cancel();
    super.dispose();
  }

  Future<void> _reload({bool silent = false}) async {
    if (!silent && mounted) setState(() => _loading = true);
    try {
      final list = await LocalDownload.list();
      if (!mounted) return;
      setState(() {
        _tasks = list;
        _loading = false;
        _status = null;
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
        return msg.isEmpty ? '排队中' : msg;
      case 'running':
        final p = (t['progress'] is num) ? (t['progress'] as num).toDouble() : 0.0;
        final pct = (p * 100).clamp(0, 100).toStringAsFixed(0);
        return msg.isEmpty ? '下载中 $pct%' : '$msg · $pct%';
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

  @override
  Widget build(BuildContext context) {
    final p = KotvPalette.of(context);
    final compact = KotvLayout.isCompact(context);
    final s = LayoutScale.of(context);
    return Column(
      children: [
        LibraryTopBar(
          title: '我的下载',
          onBack: () => kotvPageBack(ref),
          onSearch: () => goKotvPage(ref, KotvPage.search),
          onProfile: () => goKotvPage(ref, KotvPage.profile),
          onNews: () {},
          trailing: AppPill(
            label: '清理完成',
            height: compact ? 34 : 36,
            onTap: () async {
              final n = await LocalDownload.clearFinished();
              if (!mounted) return;
              setState(() => _status = n > 0 ? '已清理 $n 条' : '没有可清理项');
              await _reload();
            },
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
              : _tasks.isEmpty
                  ? Center(
                      child: Text('暂无下载任务', style: TextStyle(color: p.muted)),
                    )
                  : ListView.separated(
                      padding: EdgeInsets.fromLTRB(16 * s, 12, 16 * s, 24),
                      itemCount: _tasks.length,
                      separatorBuilder: (_, __) => SizedBox(height: 8 * s),
                      itemBuilder: (_, i) {
                        final t = _tasks[i];
                        final st = '${t['status'] ?? ''}';
                        final prog = (t['progress'] is num)
                            ? (t['progress'] as num).toDouble().clamp(0.0, 1.0)
                            : 0.0;
                        return DecoratedBox(
                          decoration: BoxDecoration(
                            color: p.pillBg,
                            borderRadius: BorderRadius.circular(12),
                            border: Border.all(color: p.pillBorder),
                          ),
                          child: Padding(
                            padding: const EdgeInsets.fromLTRB(12, 10, 8, 10),
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.stretch,
                              children: [
                                Row(
                                  children: [
                                    Expanded(
                                      child: Text(
                                        _label(t),
                                        maxLines: 2,
                                        overflow: TextOverflow.ellipsis,
                                        style: TextStyle(
                                          color: p.fg,
                                          fontSize: 14,
                                          fontWeight: FontWeight.w600,
                                        ),
                                      ),
                                    ),
                                    if (st == 'queued' || st == 'running')
                                      AppPill(
                                        label: '取消',
                                        height: 32,
                                        width: 64,
                                        onTap: () async {
                                          await LocalDownload.cancel('${t['id']}');
                                          await _reload();
                                        },
                                      )
                                    else
                                      AppPill(
                                        label: '删除',
                                        height: 32,
                                        width: 64,
                                        onTap: () async {
                                          await LocalDownload.remove('${t['id']}');
                                          await _reload();
                                        },
                                      ),
                                  ],
                                ),
                                const SizedBox(height: 6),
                                Text(
                                  _statusText(t),
                                  style: TextStyle(color: p.muted, fontSize: 12),
                                ),
                                if (st == 'running') ...[
                                  const SizedBox(height: 8),
                                  ClipRRect(
                                    borderRadius: BorderRadius.circular(4),
                                    child: LinearProgressIndicator(
                                      value: prog > 0 ? prog : null,
                                      minHeight: 4,
                                      backgroundColor: p.fg.withOpacity(0.08),
                                      color: p.primary,
                                    ),
                                  ),
                                ],
                                if ('${t['path'] ?? ''}'.trim().isNotEmpty) ...[
                                  const SizedBox(height: 4),
                                  Text(
                                    '${t['path']}',
                                    maxLines: 1,
                                    overflow: TextOverflow.ellipsis,
                                    style: TextStyle(
                                        color: p.muted.withOpacity(0.8),
                                        fontSize: 11),
                                  ),
                                ],
                              ],
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
