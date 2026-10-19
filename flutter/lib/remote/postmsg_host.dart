import 'dart:async';
import 'dart:convert';
import 'dart:math' as math;
import '../util/kotv_io.dart';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:url_launcher/url_launcher.dart';

import '../api/kotv_api.dart';
import '../player/kotv_platform.dart';
import '../theme/kotv_palette.dart';
import '../theme/layout_scale.dart';
import '../util/kotv_ui_socket.dart';
import 'kotv_ui_markup.dart';
import 'kotv_ui_view.dart';

/// 通用宿主弹窗：只认协议 [UI:] / [UI_CLOSE:]，不关心业务。
/// 脚本（JAR/JS/Py）决定内容与时机；宿主只负责：
/// - 展示 → 回报 `shown`（附带客户端 platform，供脚本按端生成 deep link）
/// - 关闭 → 回报 `closed`
///
/// platform 以 Flutter 前端为准（非引擎 OS）：iOS 连远程引擎时仍是 `ios`。
///
/// 已有窗再来 [UI:]：同窗原地换文档（不 pop），并回报旧 id `closed` + 新 id `shown`。
/// 这是 Win7 关键：关开握手在慢机上会闪死/卡死，macOS 快所以不易踩中。
/// `overlay: true` 不换掉当前窗，在上面再叠一层。点返回先关最上面那层。
class PostMsgHost {
  PostMsgHost(this.api, {required this.navigatorKey});

  /// 当前活跃宿主（页面离开时可 cancelAll 关掉声明式窗并回传 dismiss）。
  static PostMsgHost? instance;

  final KotvApi api;
  final GlobalKey<NavigatorState> navigatorKey;

  Timer? _timeout;
  final Map<String, Timer> _overlayTimers = {};
  Timer? _flush;
  KotvUiSocket? _sock;
  bool _socketRun = false;
  Future<void>? _socketLoop;
  final List<String> _inbox = [];
  final List<String> _outbox = [];

  /// 已决定要开窗 / 正在展示（含 showDialog 尚未把路由挂上的窗口期）。
  bool _sessionActive = false;

  /// showDialog 的 Future 已发出、builder 尚未赋值 dialogContext。
  bool _opening = false;

  /// 在路由挂上前就收到了关闭请求。
  bool _closedBeforeShow = false;

  /// 是否由宿主主动 pop（区分用户返回键）。
  bool _programmaticClose = false;

  /// 路由正在卸掉。这段时间新文档先记下，等回执结束再开，避免清掉新窗或 pop 到别的页面。
  bool _closingRoute = false;

  /// 各层是否已回报过 shown / closed（避免重复）。
  final Set<String> _shownIds = {};
  final Set<String> _closedIds = {};

  /// 仅用于 pop 本 dialog 路由。
  BuildContext? _dialogContext;

  String? _activeId;
  /// 曾成功挂上的会话 id（关窗后 _activeId 可能已清空，closed 仍用它）。
  String? _lifecycleId;
  final ValueNotifier<Map<String, dynamic>?> _doc = ValueNotifier(null);
  /// 叠在当前窗上的层，底到顶。
  final ValueNotifier<List<Map<String, dynamic>>> _overlays = ValueNotifier(const []);

  /// 关旧开新时暂存下一份文档（仅真正关窗后再开时使用）。
  Map<String, dynamic>? _pendingDoc;

  /// 生命周期回执单飞队列，保证同 id shown 先于 closed。
  Future<void> _replyChain = Future<void>.value();

  String _lastToast = '';
  DateTime _lastToastAt = DateTime.fromMillisecondsSinceEpoch(0);

  /// 是否有声明式窗（含 opening 窗口期）；外壳返回键优先关窗。
  bool get hasOpenDialog => _sessionActive || _opening || _dialogContext != null;

  void start() {
    instance = this;
    _socketRun = true;
    _socketLoop ??= _runSocket();
  }

  void stop() {
    _socketRun = false;
    if (instance == this) instance = null;
    final sock = _sock;
    unawaited(cancelAll(reply: true).whenComplete(() {
      if (identical(_sock, sock)) _sock = null;
      unawaited(sock?.close());
    }));
  }

  /// 软打断引擎侧占住的请求（与 PopScope / 超时 / dismissAfter 关窗一致）。
  void _interruptPending() {
    unawaited(api.cancelPending(hard: false, thunder: false));
  }

  /// 关掉当前声明式窗，并向引擎回传 dismiss。
  /// [popDialog] 保留兼容：关窗一律走 dialog context，不会误 pop 其它路由。
  /// 用户/外壳取消时顺带 [cancelPending]，避免 JAR 占住后详情再也进不去。
  Future<void> cancelAll({bool reply = true, bool popDialog = true}) async {
    final ids = <String>[
      if ((_activeId ?? _lifecycleId)?.isNotEmpty == true) (_activeId ?? _lifecycleId)!,
      for (final layer in _overlays.value)
        if ('${layer['id']}'.isNotEmpty) '${layer['id']}',
    ];
    if (reply) {
      for (final id in ids) {
        await _enqueueReply(id: id, action: 'dismiss', values: _hostClientValues());
      }
    }
    _pendingDoc = null;
    _interruptPending();
    if (popDialog || _sessionActive || _opening) {
      _dismiss();
    } else {
      _activeId = null;
      _timeout?.cancel();
      _timeout = null;
      for (final timer in _overlayTimers.values) {
        timer.cancel();
      }
      _overlayTimers.clear();
    }
  }

  Future<void> _runSocket() async {
    var waitMs = 200;
    while (_socketRun) {
      KotvUiSocket? sock;
      try {
        final spec = await api.uiSocket();
        sock = await kotvOpenUiSocket(spec.url, spec.headers);
        if (!_socketRun) {
          await sock.close();
          return;
        }
        _sock = sock;
        waitMs = 200;
        final pending = List<String>.from(_outbox);
        _outbox.clear();
        for (final text in pending) {
          sock.send(text);
        }
        await for (final frame in sock.frames) {
          if (!_socketRun || !identical(_sock, sock)) break;
          _queueFrame(frame);
        }
      } catch (_) {}
      if (sock != null && identical(_sock, sock)) _sock = null;
      if (!_socketRun) return;
      await Future<void>.delayed(Duration(milliseconds: waitMs));
      if (waitMs < 2000) waitMs += 200;
    }
  }

  void _queueFrame(String frame) {
    final text = frame.trim();
    if (text.isEmpty) return;
    _inbox.add(text);
    _flush ??= Timer(const Duration(milliseconds: 16), _flushFrames);
  }

  void _flushFrames() {
    _flush = null;
    if (_inbox.isEmpty) return;
    final msgs = List<String>.from(_inbox);
    _inbox.clear();
    final ui = <String>[];
    final closes = <String>[];
    final other = <String>[];
    for (final s in msgs) {
      if (s.startsWith('UI:')) {
        ui.add(s);
      } else if (s.startsWith('UI_CLOSE:')) {
        closes.add(s);
      } else {
        other.add(s);
      }
    }
    for (final s in closes) {
      _handle(s);
    }
    for (final s in ui) {
      _handle(s);
    }
    for (final s in other) {
      _handle(s);
    }
  }

  void _handle(String raw) {
    final msg = raw.trim();
    if (msg.isEmpty) return;

    if (msg.startsWith('UI:')) {
      final payload = msg.substring(3);
      try {
        final decoded = jsonDecode(payload);
        if (decoded is Map && '${decoded['id'] ?? ''}'.isNotEmpty) {
          _show(Map<String, dynamic>.from(decoded));
        }
      } catch (_) {}
      return;
    }

    if (msg.startsWith('UI_CLOSE:')) {
      try {
        final cmd = jsonDecode(msg.substring(9));
        if (cmd is Map && '${cmd['id'] ?? ''}'.isNotEmpty) {
          _close('${cmd['id']}');
        }
      } catch (_) {}
      return;
    }

    final now = DateTime.now();
    if (msg == _lastToast && now.difference(_lastToastAt) < const Duration(milliseconds: 800)) {
      return;
    }
    _lastToast = msg;
    _lastToastAt = now;
    final ctx = navigatorKey.currentContext;
    if (ctx == null) return;
    ScaffoldMessenger.maybeOf(ctx)?.showSnackBar(
      SnackBar(content: Text(msg), duration: const Duration(seconds: 3)),
    );
  }

  void _show(Map<String, dynamic> doc) {
    final id = '${doc['id']}'.trim();
    if (id.isEmpty) return;
    if (_closingRoute) {
      final previous = _pendingDoc;
      _pendingDoc = doc;
      final oldId = '${previous?['id'] ?? ''}'.trim();
      if (oldId.isNotEmpty && oldId != id) unawaited(_reportClosed(oldId));
      return;
    }

    if (doc['overlay'] == true && (_sessionActive || _opening) && _doc.value != null) {
      _upsertOverlay(doc);
      return;
    }
    _dropOverlays(reportClosed: true);

    // 已有窗：原地换内容（不 pop）。Win7 关开握手会闪死；macOS 快所以以前不易暴露。
    if (_sessionActive || _opening) {
      final oldId = (_activeId ?? _lifecycleId ?? '').trim();
      _pendingDoc = null;
      _closedBeforeShow = false;
      _activeId = id;
      _lifecycleId = id;
      _doc.value = doc;
      _armTimeout(doc);
      unawaited(_ackReplace(oldId: oldId, newId: id));
      return;
    }

    _pendingDoc = null;
    _activeId = id;
    _lifecycleId = id;
    _closedBeforeShow = false;
    _armTimeout(doc);
    _doc.value = doc;

    final ctx = navigatorKey.currentContext;
    if (ctx == null) {
      unawaited(_enqueueReply(id: id, action: 'closed'));
      _resetIdleState();
      return;
    }

    _sessionActive = true;
    _opening = true;
    _programmaticClose = false;
    unawaited(_openDialog(ctx));
  }

  /// 同窗换文档：先给旧 session closed，再给新 session shown。
  Future<void> _ackReplace({required String oldId, required String newId}) async {
    if (oldId.isNotEmpty && oldId != newId) {
      await _reportClosed(oldId);
    }
    if (newId.isEmpty) return;
    if (_lifecycleId != newId || _shownIds.contains(newId)) return;
    await _reportShown(newId);
  }

  void _armTimeout(Map<String, dynamic> doc) {
    _timeout?.cancel();
    final timeoutMs = (doc['timeoutMs'] is num) ? (doc['timeoutMs'] as num).toInt() : 0;
    if (timeoutMs <= 0) return;
    final id = '${doc['id']}';
    _timeout = Timer(Duration(milliseconds: timeoutMs), () {
      if (_activeId != id) return;
      unawaited(_enqueueReply(id: id, action: 'timeout'));
      // 超时关窗也要打断 pending，否则 JAR/爬虫仍占住，后续进详情会卡住。
      _interruptPending();
      _dropOverlays(reportClosed: true);
      _dismiss();
    });
  }

  void _close(String id) {
    if (_overlays.value.any((layer) => '${layer['id']}' == id)) {
      _removeOverlay(id, reportClosed: true);
      return;
    }
    if (_pendingDoc != null && '${_pendingDoc!['id']}' == id) {
      _pendingDoc = null;
    }
    final alive = !_closingRoute && (_sessionActive || _opening || _dialogContext != null);
    final current = (_activeId ?? _lifecycleId ?? '').trim();
    if (!alive || (current.isNotEmpty && current != id)) {
      unawaited(_reportClosed(id));
      return;
    }
    _dropOverlays(reportClosed: true);
    _dismiss();
  }

  void _upsertOverlay(Map<String, dynamic> doc) {
    final id = '${doc['id']}';
    final next = <Map<String, dynamic>>[
      for (final layer in _overlays.value)
        if ('${layer['id']}' != id) layer,
      doc,
    ];
    next.sort((a, b) => _zOf(a).compareTo(_zOf(b)));
    _overlays.value = next;
    _armOverlayTimeout(doc);
  }

  void _removeOverlay(String id, {required bool reportClosed}) {
    _overlayTimers.remove(id)?.cancel();
    final next = [
      for (final layer in _overlays.value)
        if ('${layer['id']}' != id) layer,
    ];
    if (next.length == _overlays.value.length) return;
    _overlays.value = next;
    if (reportClosed) unawaited(_reportClosed(id));
  }

  void _dropOverlays({required bool reportClosed}) {
    final old = _overlays.value;
    if (old.isEmpty) return;
    _overlays.value = const [];
    for (final layer in old) {
      final id = '${layer['id']}';
      _overlayTimers.remove(id)?.cancel();
      if (reportClosed) unawaited(_reportClosed(id));
    }
  }

  void _armOverlayTimeout(Map<String, dynamic> doc) {
    final id = '${doc['id']}';
    _overlayTimers.remove(id)?.cancel();
    final timeoutMs = (doc['timeoutMs'] is num) ? (doc['timeoutMs'] as num).toInt() : 0;
    if (timeoutMs <= 0) return;
    _overlayTimers[id] = Timer(Duration(milliseconds: timeoutMs), () {
      if (!_overlays.value.any((layer) => '${layer['id']}' == id)) return;
      unawaited(_enqueueReply(id: id, action: 'timeout'));
      _removeOverlay(id, reportClosed: true);
    });
  }

  void _dismiss() {
    _closingRoute = true;
    _timeout?.cancel();
    _timeout = null;
    for (final timer in _overlayTimers.values) {
      timer.cancel();
    }
    _overlayTimers.clear();
    _activeId = null;
    // 不要立刻清空 _doc：保留最后一帧直到路由卸掉，避免空 barrier 闪烁。
    _closedBeforeShow = true;
    _popDialogRoute();
    // 若 showDialog 还没挂上就取消：直接回报 closed。
    if (!_sessionActive && !_opening) {
      unawaited(_reportClosed());
    } else if (_opening && _dialogContext == null) {
      // builder 尚未跑；等 finally / 早退路径回报。
    }
  }

  void _popDialogRoute() {
    _programmaticClose = true;
    final dialogCtx = _dialogContext;
    if (dialogCtx == null || !dialogCtx.mounted) {
      _dialogContext = null;
      return;
    }
    NavigatorState? nav;
    try {
      nav = Navigator.of(dialogCtx, rootNavigator: true);
    } catch (_) {}
    if (nav != null && nav.canPop()) {
      nav.pop();
      return;
    }
    // 弹不掉：会话与路由可能失步。清 context，并若已无 opening 则强制 idle，
    // 避免 hasOpenDialog 永久为 true 导致返回键只打空转、详情再也进不去。
    _dialogContext = null;
    if (!_opening && (_sessionActive || _lifecycleId != null)) {
      final id = (_lifecycleId ?? '').trim();
      if (id.isNotEmpty && _closedIds.add(id)) {
        unawaited(_enqueueReply(id: id, action: 'closed', values: _hostClientValues()));
      }
      _resetIdleState();
    }
  }

  Future<void> _enqueueReply({
    required String id,
    required String action,
    Map<String, String>? values,
  }) {
    final done = Completer<void>();
    _replyChain = _replyChain.catchError((_) {}).then((_) async {
      try {
        final payload = jsonEncode({
          'id': id,
          'action': action,
          if (values != null && values.isNotEmpty) 'values': values,
        });
        final sock = _sock;
        if (sock == null) {
          _outbox.add(payload);
        } else {
          sock.send(payload);
        }
      } catch (_) {
        _outbox.add(jsonEncode({
          'id': id,
          'action': action,
          if (values != null && values.isNotEmpty) 'values': values,
        }));
      } finally {
        if (!done.isCompleted) done.complete();
      }
    });
    return done.future;
  }

  /// 客户端宿主信息（不是引擎机器）：脚本用 platform 决定 deep link / 按钮。
  Map<String, String> _hostClientValues([Map<String, String>? extra]) {
    final out = <String, String>{
      'platform': kotvHostPlatform(),
      'desktop': kotvIsDesktop() ? 'true' : 'false',
    };
    if (extra != null && extra.isNotEmpty) out.addAll(extra);
    return out;
  }

  Future<void> _reportShown(String id) async {
    if (id.isEmpty || !_shownIds.add(id)) return;
    await _enqueueReply(id: id, action: 'shown', values: _hostClientValues());
  }

  Future<void> _reportClosed([String? id]) async {
    final target = (id ?? _lifecycleId ?? '').trim();
    if (target.isEmpty || !_closedIds.add(target)) return;
    await _enqueueReply(id: target, action: 'closed', values: _hostClientValues());
  }

  void _resetIdleState() {
    _timeout?.cancel();
    _timeout = null;
    _activeId = null;
    _lifecycleId = null;
    _doc.value = null;
    _overlays.value = const [];
    _pendingDoc = null;
    _sessionActive = false;
    _opening = false;
    _closedBeforeShow = false;
    _closingRoute = false;
    _dialogContext = null;
    _programmaticClose = false;
    _shownIds.clear();
    _closedIds.clear();
  }

  Future<void> _openDialog(BuildContext ctx) async {
    var mountedRoute = false;

    try {
      if (_doc.value == null || _closedBeforeShow) {
        _sessionActive = false;
        _opening = false;
        await _reportClosed();
        return;
      }

      await showDialog<void>(
        context: ctx,
        useRootNavigator: true,
        barrierDismissible: false,
        builder: (dialogCtx) {
          _dialogContext = dialogCtx;
          _opening = false;
          mountedRoute = true;

          if (_closedBeforeShow) {
            WidgetsBinding.instance.addPostFrameCallback((_) {
              _popDialogRoute();
              unawaited(_reportClosed());
            });
            // 仍渲染当前文档一帧，避免空 barrier 闪烁；随即 pop。
            final doc = _doc.value;
            if (doc == null) return const SizedBox.shrink();
            return _buildDialogShell(doc);
          }

          WidgetsBinding.instance.addPostFrameCallback((_) {
            // 用回调时的当前 id：opening 窗口期内可能已原地换成新 session。
            final id = (_lifecycleId ?? _activeId ?? '').trim();
            if (!_closedBeforeShow && _doc.value != null && id.isNotEmpty) {
              unawaited(_reportShown(id));
            }
          });

          return ValueListenableBuilder<List<Map<String, dynamic>>>(
            valueListenable: _overlays,
            builder: (overlayCtx, overlays, base) {
              return PopScope(
                canPop: overlays.isEmpty,
                onPopInvoked: (didPop) {
                  if (didPop) _closingRoute = true;
                  if (!didPop) {
                    if (overlays.isEmpty) return;
                    final id = '${overlays.last['id']}';
                    unawaited(_enqueueReply(id: id, action: 'dismiss', values: _hostClientValues()));
                    _removeOverlay(id, reportClosed: true);
                    return;
                  }
                  if (!_programmaticClose) {
                    // 手势/遥控返回：只关最底那层，并打断占住的爬虫请求。
                    if (_lifecycleId != null) {
                      unawaited(_enqueueReply(id: _lifecycleId!, action: 'dismiss', values: _hostClientValues()));
                    }
                    _interruptPending();
                    _activeId = null;
                  }
                  _dialogContext = null;
                },
                child: Stack(
                  fit: StackFit.expand,
                  children: [
                    if (base != null) base,
                    for (final layer in overlays)
                      _UiLayer(
                        key: ValueKey('${layer['id']}'),
                        host: this,
                        doc: layer,
                      ),
                  ],
                ),
              );
            },
            child: ValueListenableBuilder<Map<String, dynamic>?>(
              valueListenable: _doc,
              builder: (ctx, doc, _) {
                if (doc == null) {
                  WidgetsBinding.instance.addPostFrameCallback((_) => _popDialogRoute());
                  return const SizedBox.shrink();
                }
                return _UiLayer(
                  key: ValueKey('${doc['id']}'),
                  host: this,
                  doc: doc,
                  base: true,
                );
              },
            ),
          );
        },
      );
    } finally {
      _closingRoute = true;
      final closingId = (_lifecycleId ?? '').trim();
      if (mountedRoute || _shownIds.isNotEmpty || closingId.isNotEmpty) {
        await _reportClosed(closingId.isEmpty ? null : closingId);
        for (final layer in List<Map<String, dynamic>>.from(_overlays.value)) {
          await _reportClosed('${layer['id']}');
        }
      }

      _dialogContext = null;
      _sessionActive = false;
      _opening = false;
      _closedBeforeShow = false;
      _programmaticClose = false;
      _timeout?.cancel();
      _timeout = null;
      for (final timer in _overlayTimers.values) {
        timer.cancel();
      }
      _overlayTimers.clear();
      _activeId = null;
      _doc.value = null;
      _overlays.value = const [];

      final pending = _pendingDoc;
      _pendingDoc = null;
      _lifecycleId = null;
      _shownIds.clear();
      _closedIds.clear();
      _closingRoute = false;

      if (pending != null) {
        // closed 已 await 入队后再开新窗。
        _show(pending);
      }
    }
  }

  /// 关窗瞬间仍展示最后一帧内容（无交互），避免空 barrier。
  Widget _buildDialogShell(Map<String, dynamic> doc) {
    final ctx = navigatorKey.currentContext;
    if (ctx == null) return const SizedBox.shrink();
    final p = KotvPalette.of(ctx);
    final title = '${doc['title'] ?? ''}';
    return _hostUiCard(
      context: ctx,
      doc: doc,
      title: title.isEmpty
          ? null
          : Text(title, style: TextStyle(color: p.fg, fontSize: 22, fontWeight: FontWeight.w700)),
      body: Center(child: CircularProgressIndicator(color: p.primary)),
    );
  }

  /// 设计稿 960×540，整窗（含弹窗）按屏等比缩放。
  /// 桌面仍用 [LayoutScale]（1280×720）。
  static double hostUiScale(BuildContext context) {
    final size = MediaQuery.sizeOf(context);
    if (kotvIsAndroid()) {
      final byW = size.width / 960.0;
      final byH = size.height / 540.0;
      return math.min(byW, byH).clamp(0.75, 1.55);
    }
    return LayoutScale.layoutOf(context);
  }

  int _zOf(Map<String, dynamic> doc) => kotvUiNum(doc['z'] ?? doc['zIndex']).round();

  double? _coord(Map<String, dynamic> doc, String a, String b) {
    if (doc.containsKey(a)) return kotvUiNum(doc[a]);
    if (doc.containsKey(b)) return kotvUiNum(doc[b]);
    return null;
  }

  bool _windowPlaced(Map<String, dynamic> doc) =>
      doc.containsKey('x') || doc.containsKey('y') || doc.containsKey('left') || doc.containsKey('top');

  bool _overlayModal(Map<String, dynamic> doc) {
    if (doc.containsKey('modal')) return doc['modal'] == true;
    return !_windowPlaced(doc);
  }

  Widget? _windowTitle(Map<String, dynamic> doc, Color fg, double s) {
    final title = '${doc['title'] ?? ''}';
    if (title.isEmpty) return null;
    final titleFont = kotvUiNum(doc['titleSize']);
    final resolved = (titleFont > 0 ? titleFont : (kotvIsDesktop() ? 22.0 : 20.0)) * s;
    return Text(
      title,
      style: TextStyle(color: fg, fontSize: resolved, fontWeight: FontWeight.w700, height: 1.2),
    );
  }

  Widget? _windowActions(
    BuildContext context,
    Map<String, dynamic> doc,
    double s,
    Future<void> Function(String action, {bool dismissAfter}) fire,
  ) {
    final actions = (doc['actions'] as List?) ?? const [];
    final actionMaps = [
      for (final a in actions)
        if (a is Map) Map<String, dynamic>.from(a),
    ];
    if (actionMaps.isEmpty) return null;
    final docFont = kotvUiNum(doc['fontSize']);
    final actionFont = kotvUiNum(doc['actionFontSize']);
    final actionH = kotvUiNum(doc['actionHeight']);
    return kotvUiActions(
      context: context,
      actions: actionMaps,
      align: '${doc['actionAlign'] ?? ''}',
      defaultFont: (actionFont > 0 ? actionFont : docFont) * s,
      defaultH: (actionH > 0 ? actionH : 40) * s,
      defaultW: kotvUiNum(doc['actionWidth']),
      uiScale: s,
      onPressed: (a) => fire(
        '${a['id'] ?? 'action'}',
        dismissAfter: a['dismiss'] == true,
      ),
    );
  }

  Widget _buildDocBody({
    required Map<String, dynamic> doc,
    required KotvPalette palette,
    required Color fg,
    required double uiScale,
    required Map<String, TextEditingController> values,
    required Map<String, bool> checks,
    required Map<String, String> radios,
    required Map<String, String> selects,
    required void Function(VoidCallback) setLocal,
    required Future<void> Function(String action, {bool dismissAfter}) fire,
  }) {
    final html = '${doc['html'] ?? ''}'.trim();
    final docFont = kotvUiNum(doc['fontSize']);
    final flow = <Map<String, dynamic>>[];
    final placed = <Map<String, dynamic>>[];
    for (final raw in (doc['elements'] as List?) ?? const []) {
      if (raw is! Map) continue;
      final el = Map<String, dynamic>.from(raw);
      if (_windowPlaced(el)) {
        placed.add(el);
      } else {
        flow.add(el);
      }
    }
    placed.sort((a, b) => _zOf(a).compareTo(_zOf(b)));
    List<Widget> built(Map<String, dynamic> el) => kotvUiElements(
          el,
          palette: palette,
          docFontSize: docFont,
          uiScale: uiScale,
          values: values,
          checks: checks,
          radios: radios,
          selects: selects,
          setLocal: setLocal,
          fire: fire,
          openExternal: _openExternal,
        );
    final column = Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (html.isNotEmpty)
          KotvUiMarkup(
            html: html,
            color: fg,
            fontSize: (docFont > 0 ? docFont : 14) * uiScale,
            scale: uiScale,
            onAction: (id) => unawaited(fire(id)),
            onLink: (url) => unawaited(_openExternal(url)),
          ),
        for (final el in flow) ...built(el),
      ],
    );
    if (placed.isEmpty) {
      return SingleChildScrollView(child: column);
    }
    return LayoutBuilder(
      builder: (context, cons) {
        final height = cons.maxHeight.isFinite && cons.maxHeight < double.infinity
            ? cons.maxHeight
            : 280.0 * uiScale;
        return SizedBox(
          height: height,
          child: Stack(
            clipBehavior: Clip.none,
            children: [
              Positioned.fill(child: SingleChildScrollView(child: column)),
              for (final el in placed)
                Positioned(
                  left: (_coord(el, 'x', 'left') ?? 0) * uiScale,
                  top: (_coord(el, 'y', 'top') ?? 0) * uiScale,
                  width: kotvUiNum(el['width']) > 0 ? kotvUiNum(el['width']) * uiScale : null,
                  height: kotvUiNum(el['height']) > 0 ? kotvUiNum(el['height']) * uiScale : null,
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: built(el),
                  ),
                ),
            ],
          ),
        );
      },
    );
  }

  Widget _placeWindow({
    required BuildContext context,
    required Map<String, dynamic> doc,
    required Widget card,
    required bool scrim,
    required VoidCallback? onScrim,
  }) {
    if (!_windowPlaced(doc) && !scrim) return card;
    final s = hostUiScale(context);
    final dx = _coord(doc, 'x', 'left');
    final dy = _coord(doc, 'y', 'top');
    return Stack(
      fit: StackFit.expand,
      children: [
        if (scrim)
          GestureDetector(
            behavior: HitTestBehavior.opaque,
            onTap: onScrim,
            child: const ColoredBox(color: Color(0x33000000)),
          ),
        if (dx != null || dy != null)
          Positioned(
            left: (dx ?? 0) * s,
            top: (dy ?? 0) * s,
            child: card,
          )
        else
          Center(child: card),
      ],
    );
  }

  /// 换源大卡片：半透明 dialogBg + 18 圆角描边；尺寸随 [hostUiScale] 自动缩放。
  Widget _hostUiCard({
    required BuildContext context,
    required Map<String, dynamic> doc,
    required Widget body,
    Widget? title,
    Widget? actions,
    bool asDialog = true,
  }) {
    final p = KotvPalette.of(context);
    final screen = MediaQuery.sizeOf(context);
    final desktop = kotvIsDesktop();
    final s = hostUiScale(context);
    final compact = KotvLayout.isCompact(context);
    final inset = EdgeInsets.symmetric(
      horizontal: (compact ? 10.0 : (desktop ? 64.0 : 48.0)) * s,
      vertical: (compact ? 12.0 : (desktop ? 48.0 : 40.0)) * s,
    );
    final maxW = screen.width * 0.95;
    final maxH = screen.height * 0.9;
    final rawW = kotvUiNum(doc['width']);
    final rawH = kotvUiNum(doc['height']);
    // 脚本给的 width/font 按设计稿 dp；未给时默认卡宽也按 s 缩放。
    // 脚本写了宽度就用它，不再抬到桌面默认最小宽。
    var cardW = rawW > 0 ? rawW * s : (desktop ? 560.0 : 420.0) * s;
    cardW = rawW > 0 ? math.min(cardW, maxW) : cardW.clamp(280.0 * s, maxW);
    final customPad = doc['padding'] != null;
    final pad = customPad ? kotvUiNum(doc['padding']) * s : 0.0;
    final padH = customPad ? pad : (compact ? 14.0 : 24.0) * s;
    final padVT = customPad ? pad : (compact ? 14.0 : 22.0) * s;
    final padVB = customPad ? pad : (compact ? 12.0 : 18.0) * s;
    final gapTitle = (compact ? 8.0 : 12.0) * s;
    final gapActions = (compact ? 12.0 : 16.0) * s;
    final radius = doc['radius'] != null ? kotvUiNum(doc['radius']) * s : 18.0 * s;
    final bg = kotvUiColor(doc['background']) ?? p.dialogBg;
    final border = kotvUiColor(doc['border']) ?? p.outline;

    // 内容区：可滚动 + 最大高度封顶；短内容随内容收缩（不强制撑满）。
    final bodyMaxH = math.max(
      80.0,
      maxH - padVT - padVB - (title != null ? 48 * s + gapTitle : 0) - (actions != null ? 48 * s + gapActions : 0) - 8,
    );
    final content = rawH > 0
        ? SizedBox(
            width: cardW,
            height: math.min(rawH * s, bodyMaxH),
            child: body,
          )
        : ConstrainedBox(
            constraints: BoxConstraints(maxWidth: cardW, maxHeight: bodyMaxH),
            child: body,
          );

    final card = ConstrainedBox(
        constraints: BoxConstraints(maxWidth: cardW, maxHeight: maxH),
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: bg,
            borderRadius: BorderRadius.circular(radius),
            border: Border.all(color: border),
          ),
          child: Padding(
            padding: EdgeInsets.fromLTRB(padH, padVT, padH, padVB),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (title != null) ...[
                  title,
                  SizedBox(height: gapTitle),
                ],
                content,
                if (actions != null) ...[
                  SizedBox(height: gapActions),
                  actions,
                ],
              ],
            ),
          ),
        ),
      );
    if (!asDialog) return card;
    return Dialog(
      backgroundColor: Colors.transparent,
      insetPadding: inset,
      child: card,
    );
  }

  Future<void> _openExternal(String raw) async {
    final s = raw.trim();
    if (s.isEmpty) return;
    if (Platform.isAndroid) {
      try {
        final ok = await const MethodChannel('kotv_android').invokeMethod<bool>('openView', {'url': s});
        if (ok == true) return;
      } catch (_) {}
    }
    final uri = Uri.tryParse(s.contains(':') ? s : 'package:$s');
    if (uri == null || !uri.hasScheme) return;
    if (Platform.isAndroid && (uri.scheme == 'package' || uri.scheme == 'intent')) return;
    try {
      final ok = await launchUrl(uri, mode: LaunchMode.externalApplication);
      if (ok) return;
    } catch (_) {}
    try {
      if (Platform.isMacOS) {
        await Process.run('open', [s]);
      } else if (Platform.isWindows) {
        // 勿用 `cmd /c start URL`：查询串里的 & 会被 cmd 当成命令分隔符截断。
        await Process.run('rundll32', ['url.dll,FileProtocolHandler', s]);
      } else if (Platform.isLinux) {
        await Process.run('xdg-open', [s]);
      }
    } catch (_) {}
  }

}

/// 底窗和叠层共用。叠层只关自己。
class _UiLayer extends StatefulWidget {
  const _UiLayer({super.key, required this.host, required this.doc, this.base = false});

  final PostMsgHost host;
  final Map<String, dynamic> doc;
  final bool base;

  @override
  State<_UiLayer> createState() => _UiLayerState();
}

class _UiLayerState extends State<_UiLayer> {
  final values = <String, TextEditingController>{};
  final checks = <String, bool>{};
  final radios = <String, String>{};
  final selects = <String, String>{};

  @override
  void initState() {
    super.initState();
    if (widget.base) return;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      unawaited(widget.host._reportShown('${widget.doc['id']}'));
    });
  }

  @override
  void dispose() {
    for (final controller in values.values) {
      controller.dispose();
    }
    super.dispose();
  }

  Map<String, String> _collect() {
    final out = <String, String>{};
    for (final entry in values.entries) {
      out[entry.key] = entry.value.text;
    }
    for (final entry in checks.entries) {
      out[entry.key] = entry.value ? 'true' : 'false';
    }
    out.addAll(radios);
    out.addAll(selects);
    return out;
  }

  Future<void> _fire(String action, {bool dismissAfter = false}) async {
    final host = widget.host;
    final id = '${widget.doc['id']}';
    await host._enqueueReply(id: id, action: action, values: _collect());
    if (!dismissAfter) return;
    if (widget.base) {
      host._interruptPending();
      host._dropOverlays(reportClosed: true);
      host._dismiss();
      return;
    }
    host._removeOverlay(id, reportClosed: true);
  }

  @override
  Widget build(BuildContext context) {
    final doc = widget.doc;
    final host = widget.host;
    final id = '${doc['id']}';
    final palette = KotvPalette.of(context);
    final scale = PostMsgHost.hostUiScale(context);
    final fg = kotvUiColor(doc['color']) ?? palette.fg;
    final modal = widget.base ? false : host._overlayModal(doc);
    return host._placeWindow(
      context: context,
      doc: doc,
      scrim: modal,
      onScrim: widget.base
          ? null
          : () {
              unawaited(host._enqueueReply(id: id, action: 'dismiss', values: host._hostClientValues()));
              host._removeOverlay(id, reportClosed: true);
            },
      card: host._hostUiCard(
        context: context,
        doc: doc,
        asDialog: !host._windowPlaced(doc),
        title: host._windowTitle(doc, fg, scale),
        body: host._buildDocBody(
          doc: doc,
          palette: palette,
          fg: fg,
          uiScale: scale,
          values: values,
          checks: checks,
          radios: radios,
          selects: selects,
          setLocal: setState,
          fire: _fire,
        ),
        actions: host._windowActions(context, doc, scale, _fire),
      ),
    );
  }
}
