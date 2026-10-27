import 'package:flutter/material.dart';

import '../theme/kotv_palette.dart';

/// 短列表也能下拉（配合 [RefreshIndicator]）。
const kotvPullRefreshPhysics = AlwaysScrollableScrollPhysics();

/// 空/加载占位：可滚动，便于触发下拉刷新。
Widget kotvPullRefreshFill(Widget child, {double minHeight = 280}) {
  return LayoutBuilder(
    builder: (context, c) {
      final h = c.maxHeight.isFinite && c.maxHeight > minHeight
          ? c.maxHeight
          : minHeight;
      return ListView(
        physics: kotvPullRefreshPhysics,
        children: [
          SizedBox(height: h, child: child),
        ],
      );
    },
  );
}

/// 统一下拉刷新外观；[child] 须为可滚动且建议带 [kotvPullRefreshPhysics]。
class KotvPullRefresh extends StatelessWidget {
  const KotvPullRefresh({
    super.key,
    required this.onRefresh,
    required this.child,
  });

  final Future<void> Function() onRefresh;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return RefreshIndicator(
      onRefresh: onRefresh,
      color: KotvPalette.of(context).primary,
      child: child,
    );
  }
}
