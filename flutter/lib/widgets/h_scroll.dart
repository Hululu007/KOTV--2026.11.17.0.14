import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';

/// 桌面端允许鼠标拖动滚动（Flutter 默认仅触摸/触控板）。
class KotvScrollBehavior extends MaterialScrollBehavior {
  const KotvScrollBehavior({this.showScrollbar = true});

  final bool showScrollbar;

  @override
  Set<PointerDeviceKind> get dragDevices => {
        PointerDeviceKind.touch,
        PointerDeviceKind.mouse,
        PointerDeviceKind.trackpad,
        PointerDeviceKind.stylus,
        PointerDeviceKind.unknown,
      };

  @override
  Widget buildScrollbar(BuildContext context, Widget child, ScrollableDetails details) {
    if (!showScrollbar) return child;
    return super.buildScrollbar(context, child, details);
  }
}

/// 横向列表：鼠标拖动 + 滚轮（上下也映射为左右）。
/// 打开时停在 [scrollToIndex]。选中项变化且落在视口外时拉进来。
/// 用户自己滑动之后，选中项可以离开视口，直到下一次选中变化。
/// [itemStride] 已知则按步长计算；宽度随文字变化时按子组件布局定位。
class HScrollList extends StatefulWidget {
  const HScrollList({
    super.key,
    required this.itemCount,
    required this.itemBuilder,
    this.separatorBuilder,
    this.padding,
    this.physics,
    this.scrollToIndex,
    this.itemStride,
  });

  final int itemCount;
  final NullableIndexedWidgetBuilder itemBuilder;
  final IndexedWidgetBuilder? separatorBuilder;
  final EdgeInsetsGeometry? padding;
  final ScrollPhysics? physics;
  final int? scrollToIndex;
  /// 每项占位（含间距）。为空时按子组件布局结果定位。
  final double? itemStride;

  @override
  State<HScrollList> createState() => _HScrollListState();
}

class _HScrollListState extends State<HScrollList> {
  late final KotvJumpController _sc = KotvJumpController(initialOffset: _offset(widget.scrollToIndex));
  final GlobalKey _selKey = GlobalKey();
  int? _pendingIndex;
  var _hideUntilPlaced = false;
  var _placeTries = 0;
  var _placed = false;
  int? _keepIndex;
  var _keepTries = 0;
  var _keepScheduled = false;
  var _userOwnsOffset = false;
  final Map<int, BuildContext> _itemCtx = {};

  bool get _measure =>
      !_placed && widget.itemStride == null && widget.scrollToIndex != null && widget.scrollToIndex! > 0;

  double _offset(int? index) {
    final stride = widget.itemStride;
    if (index == null || index <= 0 || stride == null || stride <= 0) return 0;
    return index * stride;
  }

  @override
  void initState() {
    super.initState();
    final index = widget.scrollToIndex;
    if (_measure) {
      _hideUntilPlaced = true;
      _queuePlace(index);
    } else {
      _placed = true;
    }
  }

  @override
  void didUpdateWidget(HScrollList oldWidget) {
    super.didUpdateWidget(oldWidget);
    final index = widget.scrollToIndex;
    if (_placed) {
      if (index != oldWidget.scrollToIndex) {
        _userOwnsOffset = false;
        _scheduleKeepVisible(index);
      }
      return;
    }
    // 打开时把正在用的项放进视口。
    if (index == null || index <= 0) return;
    if (widget.itemStride != null) {
      _pendingIndex = null;
      _hideUntilPlaced = false;
      _placed = true;
      _sc.reveal(_offset(index));
      return;
    }
    _queuePlace(index);
  }

  void _scheduleKeepVisible(int? index, {bool reset = true}) {
    if (reset) {
      _keepIndex = index;
      _keepTries = 0;
    }
    final target = _keepIndex;
    if (target == null || target < 0 || target >= widget.itemCount) return;
    if (_keepScheduled) return;
    _keepScheduled = true;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _keepScheduled = false;
      _keepVisible();
    });
  }

  void _releaseToUser() {
    _userOwnsOffset = true;
    _keepIndex = null;
  }

  void _keepVisible() {
    final index = _keepIndex;
    if (!mounted || _userOwnsOffset || index == null || index != widget.scrollToIndex) return;
    // 用户拖动/甩动时方向不是 idle；程序滚动（jumpTo/animateTo）保持 idle。
    if (_sc.hasClients && _sc.position.userScrollDirection != ScrollDirection.idle) {
      _releaseToUser();
      return;
    }
    if (index < 0 || index >= widget.itemCount) {
      _keepIndex = null;
      return;
    }
    if (_keepByStride(index)) {
      _keepIndex = null;
      return;
    }
    final ctx = _itemCtx[index];
    if (ctx != null && ctx.mounted && kotvJumpIntoViewIfClipped(ctx)) {
      _keepIndex = null;
      return;
    }
    if (_keepTries++ > 40 || !_sc.hasClients || !_sc.position.hasContentDimensions) {
      if (_keepTries > 40) {
        _keepIndex = null;
      } else {
        _scheduleKeepVisible(index, reset: false);
      }
      return;
    }
    final pos = _sc.position;
    final lo = _ctxEdge(low: true);
    final hi = _ctxEdge(low: false);
    final view = pos.viewportDimension;
    if (lo == null || hi == null || view <= 0 || (index >= lo && index <= hi)) {
      if (_keepTries > 40) {
        _keepIndex = null;
      } else {
        _scheduleKeepVisible(index, reset: false);
      }
      return;
    }
    final dir = index < lo ? -1.0 : 1.0;
    final target = (pos.pixels + dir * view * 0.85).clamp(pos.minScrollExtent, pos.maxScrollExtent).toDouble();
    if ((target - pos.pixels).abs() < 1) {
      _keepIndex = null;
      return;
    }
    pos.jumpTo(target);
    _scheduleKeepVisible(index, reset: false);
  }

  int? _ctxEdge({required bool low}) {
    if (_itemCtx.isEmpty) return null;
    var v = _itemCtx.keys.first;
    for (final i in _itemCtx.keys) {
      if (low ? i < v : i > v) v = i;
    }
    return v;
  }

  /// 已知每项步长时，只在该项超出视口时平移。返回 false 表示还不能按步长判断。
  bool _keepByStride(int index) {
    final stride = widget.itemStride;
    if (stride == null || stride <= 0) return false;
    if (!_sc.hasClients || !_sc.position.hasContentDimensions) return false;
    final pos = _sc.position;
    final start = index * stride;
    final end = start + stride;
    final lead = pos.pixels;
    final trail = lead + pos.viewportDimension;
    final double target;
    if (start < lead) {
      target = start;
    } else if (end > trail) {
      target = end - pos.viewportDimension;
    } else {
      return true;
    }
    final clamped = target.clamp(pos.minScrollExtent, pos.maxScrollExtent).toDouble();
    if ((pos.pixels - clamped).abs() >= 0.5) pos.jumpTo(clamped);
    return true;
  }

  void _queuePlace(int? index) {
    if (index == null || index <= 0 || widget.itemStride != null) {
      _pendingIndex = null;
      _hideUntilPlaced = false;
      return;
    }
    _pendingIndex = index;
    _placeTries = 0;
    WidgetsBinding.instance.addPostFrameCallback((_) => _placePending());
  }

  void _placePending() {
    final index = _pendingIndex;
    if (!mounted || index == null || index != widget.scrollToIndex) return;
    final object = _selKey.currentContext?.findRenderObject();
    final viewport = object == null ? null : RenderAbstractViewport.maybeOf(object);
    if (object is! RenderBox || !object.hasSize || viewport == null || !_sc.hasClients || !_sc.position.hasContentDimensions) {
      if (_placeTries++ < 4) {
        WidgetsBinding.instance.addPostFrameCallback((_) => _placePending());
        return;
      }
      _pendingIndex = null;
      _hideUntilPlaced = false;
      _placed = true;
      if (mounted) setState(() {});
      return;
    }
    final target = viewport.getOffsetToReveal(object, 0.0).offset;
    _pendingIndex = null;
    _hideUntilPlaced = false;
    _placed = true;
    _sc.reveal(target);
    if (mounted) setState(() {});
  }

  @override
  void dispose() {
    _sc.dispose();
    super.dispose();
  }

  Widget? _buildItem(BuildContext context, int i) {
    final child = widget.itemBuilder(context, i);
    if (child == null) return null;
    // 初次定位才套一把钥匙，量完即拆。钥匙跟着选中项挪会把焦点一起带走。
    final body = (!_measure || _placed || i != widget.scrollToIndex)
        ? child
        : KeyedSubtree(key: _selKey, child: child);
    return _HItemAnchor(index: i, host: this, child: body);
  }

  bool _onUserScroll(ScrollNotification n) {
    if (n.metrics.axis != Axis.horizontal) return false;
    final dragged = n is ScrollStartNotification
        ? n.dragDetails != null
        : n is ScrollUpdateNotification
            ? n.dragDetails != null
            : n is OverscrollNotification && n.dragDetails != null;
    if (dragged) _releaseToUser();
    return false;
  }

  void _onWheel(PointerSignalEvent e) {
    if (e is! PointerScrollEvent || !_sc.hasClients) return;
    final delta = e.scrollDelta.dx != 0 ? e.scrollDelta.dx : e.scrollDelta.dy;
    if (delta == 0) return;
    _releaseToUser();
    final pos = _sc.position;
    _sc.jumpTo((_sc.offset + delta).clamp(pos.minScrollExtent, pos.maxScrollExtent));
  }

  @override
  Widget build(BuildContext context) {
    final list = widget.separatorBuilder != null
        ? ListView.separated(
            controller: _sc,
            scrollDirection: Axis.horizontal,
            cacheExtent: _measure ? 1000000 : null,
            padding: widget.padding,
            physics: widget.physics,
            itemCount: widget.itemCount,
            separatorBuilder: widget.separatorBuilder!,
            itemBuilder: _buildItem,
          )
        : ListView.builder(
            controller: _sc,
            scrollDirection: Axis.horizontal,
            cacheExtent: _measure ? 1000000 : null,
            padding: widget.padding,
            physics: widget.physics,
            itemCount: widget.itemCount,
            itemBuilder: _buildItem,
          );

    return ScrollConfiguration(
      behavior: const KotvScrollBehavior(showScrollbar: false),
      child: NotificationListener<ScrollNotification>(
        onNotification: _onUserScroll,
        child: Listener(
        onPointerSignal: _onWheel,
        child: IgnorePointer(
          ignoring: _hideUntilPlaced,
          child: Opacity(
            opacity: _hideUntilPlaced ? 0 : 1,
            child: list,
          ),
        ),
      ),
      ),
    );
  }
}

class _HItemAnchor extends StatefulWidget {
  const _HItemAnchor({required this.index, required this.host, required this.child});

  final int index;
  final _HScrollListState host;
  final Widget child;

  @override
  State<_HItemAnchor> createState() => _HItemAnchorState();
}

class _HItemAnchorState extends State<_HItemAnchor> {
  @override
  void initState() {
    super.initState();
    widget.host._itemCtx[widget.index] = context;
  }

  @override
  void didUpdateWidget(_HItemAnchor oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.host == widget.host && oldWidget.index == widget.index) return;
    if (oldWidget.host._itemCtx[oldWidget.index] == context) {
      oldWidget.host._itemCtx.remove(oldWidget.index);
    }
    widget.host._itemCtx[widget.index] = context;
  }

  @override
  void dispose() {
    if (widget.host._itemCtx[widget.index] == context) {
      widget.host._itemCtx.remove(widget.index);
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => widget.child;
}

/// 最近一层滚动区里已经完整露出就不动；被裁切时立刻跳到刚好露出。
/// 不带动外层滚动，也不做一段滚动动画。
bool kotvJumpIntoViewIfClipped(BuildContext context) {
  final object = context.findRenderObject();
  if (object is! RenderBox || !object.attached || !object.hasSize) return false;
  final viewport = RenderAbstractViewport.maybeOf(object);
  final scrollable = Scrollable.maybeOf(context);
  if (viewport == null || scrollable == null) return false;
  final position = scrollable.position;
  if (!position.hasContentDimensions) return false;
  try {
    final revealed = RevealedOffset.clampOffset(
      leadingEdgeOffset: viewport.getOffsetToReveal(object, 0.0, axis: position.axis),
      trailingEdgeOffset: viewport.getOffsetToReveal(object, 1.0, axis: position.axis),
      currentOffset: position.pixels,
    );
    if (revealed == null) return true;
    final target = revealed.offset.clamp(position.minScrollExtent, position.maxScrollExtent).toDouble();
    if (target != position.pixels) position.jumpTo(target);
    return true;
  } catch (_) {
    return false;
  }
}

/// 列表打开时直接停在指定偏移，不从开头滑过去。
class KotvJumpController extends ScrollController {
  KotvJumpController({double initialOffset = 0})
      : _offset = initialOffset < 0 ? 0 : initialOffset,
        super(initialScrollOffset: initialOffset < 0 ? 0 : initialOffset, keepScrollOffset: false);

  double _offset;

  void arm(double offset) {
    _offset = offset < 0 ? 0 : offset;
  }

  void reveal(double offset) {
    arm(offset);
    if (!hasClients || !position.hasContentDimensions) return;
    final pos = position;
    final target = _offset.clamp(pos.minScrollExtent, pos.maxScrollExtent);
    if ((pos.pixels - target).abs() < 0.5) return;
    pos.jumpTo(target);
  }

  @override
  ScrollPosition createScrollPosition(ScrollPhysics physics, ScrollContext context, ScrollPosition? oldPosition) {
    return ScrollPositionWithSingleContext(
      physics: physics,
      context: context,
      initialPixels: _offset,
      keepScrollOffset: keepScrollOffset,
      oldPosition: oldPosition,
      debugLabel: debugLabel,
    );
  }
}

/// 包装已有横向 [ListView] / [SingleChildScrollView]，仅补桌面拖动（无自管 controller）。
class HScroll extends StatelessWidget {
  const HScroll({super.key, required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) {
    return ScrollConfiguration(
      behavior: const KotvScrollBehavior(showScrollbar: false),
      child: child,
    );
  }
}
