import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../providers.dart';
import '../theme/kotv_palette.dart';
import 'chrome.dart';
import 'dialogs.dart';

/// 同时检测的站源数。
const _kCheckConcurrency = 4;

/// fail：站源首页取不到（计入失效）；error：请求本身出错（不计入失效，不会被一键禁用）。
enum _CheckState { pending, checking, pass, fail, error }

class _SpiderRow {
  _SpiderRow({required this.key, required this.name, required this.disabled});

  final String key;
  final String name;
  bool disabled;
  _CheckState state = _CheckState.pending;
  String message = '';
}

/// 检测爬虫：先列出全部站源，逐个检测打勾；可单个或一键禁用失效站源。
/// 禁用的站不进换源列表与搜索，仍在这里显示并可恢复。
Future<void> showSpiderCheckDialog(BuildContext context, WidgetRef ref) async {
  final api = ref.read(apiProvider);
  final List<_SpiderRow> rows;
  final bool checkable;
  try {
    final data = await api.tools('spiderList');
    checkable = data['checkable'] != false;
    rows = [
      for (final e in ((data['sites'] as List?) ?? const []).whereType<Map>())
        _SpiderRow(
          key: '${e['key'] ?? ''}',
          name: '${e['name'] ?? e['key'] ?? ''}',
          disabled: e['disabled'] == true,
        ),
    ];
  } catch (e) {
    if (context.mounted) showAppNews(context, '读取站源失败: $e');
    return;
  }
  if (!context.mounted) return;
  if (rows.isEmpty) {
    showAppNews(context, '暂无站源');
    return;
  }

  var open = true;
  var changed = false;
  void Function(VoidCallback)? refresh;
  // 状态先写入；弹窗已构建且未关闭时再刷新（检测可能早于首帧完成）。
  void update(VoidCallback fn) {
    fn();
    if (open) refresh?.call(() {});
  }

  Future<void> checkAll() async {
    var next = 0;
    Future<void> worker() async {
      while (open && next < rows.length) {
        final row = rows[next++];
        update(() => row.state = _CheckState.checking);
        try {
          final r = await api.tools('checkSpider', {'key': row.key});
          update(() {
            row.state = r['pass'] == true ? _CheckState.pass : _CheckState.fail;
            row.message = '${r['message'] ?? ''}'.trim();
          });
        } catch (e) {
          update(() {
            row.state = _CheckState.error;
            row.message = '$e';
          });
        }
      }
    }

    await Future.wait([for (var i = 0; i < _kCheckConcurrency; i++) worker()]);
  }

  Future<void> setDisabled(List<_SpiderRow> targets, bool disabled) async {
    if (targets.isEmpty) return;
    try {
      await api.tools('setSpidersDisabled', {
        'keys': [for (final r in targets) r.key],
        'disabled': disabled,
      });
      changed = true;
      update(() {
        for (final r in targets) {
          r.disabled = disabled;
        }
      });
    } catch (e) {
      if (context.mounted) showAppNews(context, '操作失败: $e');
    }
  }

  if (checkable) unawaited(checkAll());
  await showDialog<void>(
    context: context,
    builder: (ctx) {
      final m = kotvDialogMetrics(ctx);
      return StatefulBuilder(
        builder: (ctx, setLocal) {
          refresh = setLocal;
          final pal = KotvPalette.of(ctx);
          int count(_CheckState s) => rows.where((r) => r.state == s).length;
          final passed = count(_CheckState.pass);
          final errors = count(_CheckState.error);
          final failed =
              rows.where((r) => r.state == _CheckState.fail).toList();
          final done = passed + failed.length + errors;
          final failedEnabled = failed.where((r) => !r.disabled).toList();
          final summary = !checkable
              ? '当前设备无法检测，仍可禁用或恢复站源'
              : done < rows.length
                  ? '检测中 $done/${rows.length} · 失败 ${failed.length}'
                  : '检测完成：成功 $passed / 失败 ${failed.length}'
                      '${errors > 0 ? ' / 出错 $errors' : ''}';
          final pillH = m.compact ? 34.0 : 38.0;
          return Dialog(
            backgroundColor: Colors.transparent,
            insetPadding: m.inset,
            child: SizedBox(
              width: m.width,
              height: m.height,
              child: DecoratedBox(
                decoration: BoxDecoration(
                  color: pal.dialogBg,
                  borderRadius: BorderRadius.circular(18 * m.ui),
                  border: Border.all(color: pal.outline),
                ),
                child: Padding(
                  padding: EdgeInsets.fromLTRB(
                    m.compact ? 14 : 28,
                    m.compact ? 14 : 24,
                    m.compact ? 14 : 28,
                    m.compact ? 12 : 24,
                  ),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      Text(
                        '检测爬虫',
                        style: TextStyle(
                          color: pal.fg,
                          fontSize: m.compact ? 20 : 26,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      SizedBox(height: m.compact ? 4 : 6),
                      Text(
                        summary,
                        style: TextStyle(
                            color: pal.muted, fontSize: m.compact ? 12 : 14),
                      ),
                      SizedBox(height: m.compact ? 8 : 12),
                      Expanded(
                        child: ListView.separated(
                          itemCount: rows.length,
                          separatorBuilder: (_, __) =>
                              SizedBox(height: m.compact ? 4 : 6),
                          itemBuilder: (_, i) => _SpiderRowView(
                            row: rows[i],
                            compact: m.compact,
                            pillH: pillH,
                            onToggle: () =>
                                setDisabled([rows[i]], !rows[i].disabled),
                          ),
                        ),
                      ),
                      SizedBox(height: m.compact ? 8 : 12),
                      Row(
                        children: [
                          Expanded(
                            child: AppPill(
                              label: failedEnabled.isEmpty
                                  ? '禁用失效'
                                  : '禁用失效（${failedEnabled.length}）',
                              height: pillH + 4,
                              selected: failedEnabled.isNotEmpty,
                              onTap: () => setDisabled(failedEnabled, true),
                            ),
                          ),
                          const SizedBox(width: 10),
                          Expanded(
                            child: AppPill(
                              label: '关闭',
                              height: pillH + 4,
                              onTap: () => Navigator.pop(ctx),
                            ),
                          ),
                        ],
                      ),
                    ],
                  ),
                ),
              ),
            ),
          );
        },
      );
    },
  );
  open = false;
  if (changed) ref.invalidate(configProvider);
}

class _SpiderRowView extends StatelessWidget {
  const _SpiderRowView({
    required this.row,
    required this.compact,
    required this.pillH,
    required this.onToggle,
  });

  final _SpiderRow row;
  final bool compact;
  final double pillH;
  final VoidCallback onToggle;

  @override
  Widget build(BuildContext context) {
    final pal = KotvPalette.of(context);
    final iconSz = compact ? 18.0 : 22.0;
    final Widget status = switch (row.state) {
      _CheckState.pending =>
        Icon(Icons.schedule, size: iconSz, color: pal.muted),
      _CheckState.checking => SizedBox(
          width: iconSz,
          height: iconSz,
          child: CircularProgressIndicator(strokeWidth: 2, color: pal.primary),
        ),
      _CheckState.pass =>
        Icon(Icons.check_circle, size: iconSz, color: Colors.green),
      _CheckState.fail =>
        Icon(Icons.cancel, size: iconSz, color: Colors.redAccent),
      _CheckState.error =>
        Icon(Icons.error_outline, size: iconSz, color: Colors.orange),
    };
    final nameColor = row.disabled ? pal.muted : pal.fg;
    return Row(
      children: [
        status,
        SizedBox(width: compact ? 8 : 12),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                row.disabled ? '${row.name}（已禁用）' : row.name,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
                style: TextStyle(
                  color: nameColor,
                  fontSize: compact ? 14 : 16,
                  fontWeight: FontWeight.w600,
                ),
              ),
              if ((row.state == _CheckState.fail ||
                      row.state == _CheckState.error) &&
                  row.message.isNotEmpty)
                Text(
                  row.message,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style:
                      TextStyle(color: pal.muted, fontSize: compact ? 11 : 12),
                ),
            ],
          ),
        ),
        SizedBox(width: compact ? 8 : 12),
        AppPill(
          label: row.disabled ? '恢复' : '禁用',
          width: compact ? 64 : 80,
          height: pillH,
          fontSize: compact ? 13 : 14,
          selected: row.disabled,
          onTap: onToggle,
        ),
      ],
    );
  }
}
