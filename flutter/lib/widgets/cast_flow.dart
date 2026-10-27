import 'dart:async';

import '../util/kotv_io.dart';

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../api/kotv_api.dart';
import '../theme/kotv_palette.dart';
import '../theme/stable_focus.dart';
import 'chrome.dart';

const _androidChannel = MethodChannel('kotv_android');

String _castDeviceKey(Map d) {
  final protocol = '${d['protocol'] ?? ''}'.trim();
  final name = '${d['name'] ?? ''}'.trim();
  final detail = '${d['detail'] ?? ''}'.trim();
  return '$protocol|$name|$detail';
}

String _castDeviceLabel(Map d) {
  final label = '${d['label'] ?? ''}'.trim();
  if (label.isNotEmpty) return label;
  final name = '${d['name'] ?? ''}'.trim();
  return name.isEmpty ? '未知设备' : name;
}

/// 高版本 Android 投屏前申请附近设备 / 定位（组播发现）权限。
Future<bool> _ensureCastPermissions() async {
  if (kIsWeb || !Platform.isAndroid) return true;
  try {
    final ok = await _androidChannel.invokeMethod<bool>('ensureCastPermissions');
    return ok ?? true;
  } catch (_) {
    return true;
  }
}

/// 正在播放时投屏：先弹窗，窗内搜索并列出设备；投上后暂停本机，避免两边一起响。
Future<void> kotvCastPlaying(
  BuildContext context,
  KotvApi api, {
  required String playUrl,
  required int positionMs,
  required bool Function() isMounted,
  required void Function(String status) setStatus,
  required Future<void> Function() pauseLocal,
}) async {
  void status(String m) {
    if (isMounted()) setStatus(m);
  }

  if (playUrl.isEmpty) {
    if (isMounted()) showAppNews(context, '请先播放内容再投屏');
    return;
  }
  if (!await _ensureCastPermissions()) {
    status('需要附近设备/定位权限才能搜索投屏');
    if (context.mounted) {
      showAppNews(context, '投屏需要附近 Wi‑Fi 设备权限（Android 13+）或定位权限（旧版）。\n请在系统设置中允许后重试。');
    }
    return;
  }
  if (!context.mounted) return;
  final result = await showDialog<_CastResult>(
    context: context,
    useRootNavigator: true,
    barrierDismissible: true,
    builder: (_) => _CastDeviceDialog(api: api, onStatus: status, positionMs: positionMs),
  );
  if (result == null) {
    status('已取消投屏');
    return;
  }
  status(result.message);
  if (result.casted) unawaited(pauseLocal());
}

/// 弹窗结果：[casted] 为真表示已投到设备（本机应暂停）；断开投屏时为假。
class _CastResult {
  const _CastResult(this.message, {required this.casted});
  final String message;
  final bool casted;
}

class _CastDeviceDialog extends StatefulWidget {
  const _CastDeviceDialog({
    required this.api,
    required this.onStatus,
    required this.positionMs,
  });

  final KotvApi api;
  final void Function(String msg) onStatus;
  final int positionMs;

  @override
  State<_CastDeviceDialog> createState() => _CastDeviceDialogState();
}

class _CastDeviceDialogState extends State<_CastDeviceDialog> {
  List<Map<String, dynamic>> _devices = const [];
  bool _searching = true;
  String? _searchError;
  String _activeKey = '';
  String _activeLabel = '';
  int? _busyIndex;
  int? _statusIndex;
  String? _rowStatus;
  bool _rowOk = false;
  bool _blockPop = false;
  bool _stopping = false;

  @override
  void initState() {
    super.initState();
    unawaited(_discover());
  }

  Future<void> _discover() async {
    setState(() {
      _searching = true;
      _searchError = null;
      _statusIndex = null;
      _rowStatus = null;
    });
    widget.onStatus('正在搜索投屏设备…');
    try {
      final results = await Future.wait([
        widget.api.tools('castDiscover'),
        widget.api
            .tools('castStatus')
            .catchError((_) => <String, dynamic>{'ok': true, 'active': false}),
      ]);
      if (!mounted) return;
      final session = results[1];
      final active = session['active'] == true;
      setState(() {
        _searching = false;
        _devices = ((results[0]['devices'] as List?) ?? const [])
            .whereType<Map>()
            .map((e) => Map<String, dynamic>.from(e))
            .toList();
        _activeKey = active ? '${session['key'] ?? ''}'.trim() : '';
        _activeLabel =
            active ? '${session['label'] ?? session['name'] ?? ''}'.trim() : '';
      });
      widget.onStatus(_devices.isEmpty ? '未发现投屏设备' : '发现 ${_devices.length} 个设备');
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _searching = false;
        _searchError = '$e';
      });
      widget.onStatus('搜索失败：$e');
    }
  }

  Future<void> _castAt(int i) async {
    if (_blockPop || _stopping) return;
    final d = _devices[i];
    final index = int.tryParse('${d['index']}') ?? i;
    final label = _castDeviceLabel(d);
    setState(() {
      _blockPop = true;
      _busyIndex = i;
      _statusIndex = i;
      _rowStatus = '投屏中…';
      _rowOk = false;
    });
    widget.onStatus('正在投屏到 $label…');
    try {
      final params = <String, dynamic>{'index': index};
      if (widget.positionMs > 0) params['positionMs'] = widget.positionMs;
      final cast = await widget.api.tools('cast', params);
      final msg = '${cast['message'] ?? '已投屏到 $label'}';
      final key = '${cast['key'] ?? ''}'.trim().isNotEmpty
          ? '${cast['key']}'.trim()
          : _castDeviceKey({
              'protocol': cast['protocol'] ?? d['protocol'],
              'name': cast['name'] ?? d['name'],
              'detail': cast['detail'] ?? d['detail'],
            });
      final statusIdx = _devices.indexWhere((e) => _castDeviceKey(e) == key);
      if (!mounted) return;
      setState(() {
        _busyIndex = null;
        _statusIndex = statusIdx >= 0 ? statusIdx : i;
        _rowStatus = '已投屏';
        _rowOk = true;
        _activeKey = key;
        _activeLabel = '${cast['label'] ?? label}'.trim();
      });
      widget.onStatus(msg);
      await Future<void>.delayed(const Duration(milliseconds: 700));
      if (mounted) {
        Navigator.of(context, rootNavigator: true).pop(_CastResult(msg, casted: true));
      }
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _blockPop = false;
        _busyIndex = null;
        _statusIndex = i;
        _rowStatus = '失败：$e';
        _rowOk = false;
      });
      widget.onStatus('$e');
    }
  }

  Future<void> _stopCast() async {
    if (_blockPop || _stopping) return;
    final activeIdx = _devices.indexWhere((e) => _castDeviceKey(e) == _activeKey);
    setState(() {
      _stopping = true;
      _blockPop = true;
      _rowStatus = '断开中…';
      _rowOk = false;
      if (activeIdx >= 0) _statusIndex = activeIdx;
    });
    widget.onStatus('正在断开投屏…');
    try {
      final r = await widget.api.tools('castStop');
      final msg = '${r['message'] ?? '已断开投屏'}';
      if (!mounted) return;
      widget.onStatus(msg);
      Navigator.of(context, rootNavigator: true).pop(_CastResult(msg, casted: false));
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _stopping = false;
        _blockPop = false;
        _rowStatus = '断开失败：$e';
        _rowOk = false;
      });
      widget.onStatus('$e');
    }
  }

  Widget _action(String label, VoidCallback? onPressed, KotvPalette p, {bool autofocus = false}) {
    return KotvStableFocus(
      autofocus: autofocus,
      onPressed: onPressed,
      borderRadius: 8,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
        child: Text(label, style: TextStyle(color: p.primary, fontWeight: FontWeight.w700)),
      ),
    );
  }

  Widget _note(String text, KotvPalette p) => Padding(
        padding: const EdgeInsets.fromLTRB(24, 8, 24, 12),
        child: Text(text, style: TextStyle(color: p.muted, fontSize: 13)),
      );

  @override
  Widget build(BuildContext context) {
    final p = KotvPalette.of(context);
    final hasActive = _activeKey.isNotEmpty;
    final activeInList = hasActive && _devices.any((d) => _castDeviceKey(d) == _activeKey);

    return PopScope(
      canPop: !_blockPop,
      child: SimpleDialog(
        backgroundColor: p.dialogBg,
        title: Text('投屏到', style: TextStyle(color: p.fg)),
        children: [
          if (_searching)
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 8, 24, 16),
              child: Row(
                children: [
                  SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2, color: p.primary),
                  ),
                  const SizedBox(width: 12),
                  Text('正在搜索投屏设备…', style: TextStyle(color: p.muted)),
                ],
              ),
            )
          else ...[
            if (hasActive) ...[
              _note(activeInList ? '当前投屏：$_activeLabel' : '当前投屏：$_activeLabel（未在列表中）', p),
              _action(
                _stopping ? '断开中…' : '断开投屏',
                _blockPop ? null : () => unawaited(_stopCast()),
                p,
                autofocus: _devices.isEmpty,
              ),
              const Divider(height: 12),
            ],
            if (_searchError != null)
              _note('搜索失败：$_searchError', p)
            else if (_devices.isEmpty)
              _note('未发现设备。已搜索 DLNA / Chromecast / 局域网 App，\n请确认设备与本机在同一局域网。', p),
            for (var i = 0; i < _devices.length; i++) _deviceRow(i, p, hasActive),
            _action(
              '重新搜索',
              _blockPop ? null : () => unawaited(_discover()),
              p,
              autofocus: _devices.isEmpty && !hasActive,
            ),
          ],
        ],
      ),
    );
  }

  Widget _deviceRow(int i, KotvPalette p, bool hasActive) {
    final d = _devices[i];
    final label = _castDeviceLabel(d);
    final isActive = hasActive && _castDeviceKey(d) == _activeKey;
    final showStatus = _statusIndex == i && _rowStatus != null;
    final trailing = showStatus ? _rowStatus! : (isActive ? '投屏中' : '');
    final muted = _blockPop && _busyIndex != i;
    return KotvStableFocus(
      autofocus: i == 0 && !_blockPop && !hasActive,
      scrollIntoView: true,
      onPressed: _blockPop ? null : () => unawaited(_castAt(i)),
      borderRadius: 8,
      child: Opacity(
        opacity: muted ? 0.45 : 1,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
          child: Row(
            children: [
              Expanded(
                child: Text(
                  label,
                  style: TextStyle(
                    color: p.fg,
                    fontWeight: isActive || showStatus ? FontWeight.w700 : FontWeight.w500,
                  ),
                ),
              ),
              if (trailing.isNotEmpty)
                Flexible(
                  child: Text(
                    trailing,
                    textAlign: TextAlign.right,
                    maxLines: 2,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      color: showStatus ? (_rowOk ? const Color(0xFF5CDB95) : p.primary) : p.primary,
                      fontSize: 12.5,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}
