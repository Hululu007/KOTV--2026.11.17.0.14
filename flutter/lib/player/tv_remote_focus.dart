import 'package:flutter/widgets.dart';

/// 播控栏可见时记下上次可焦节点；失焦后方向键恢复用。
class KotvRemoteFocusMemory {
  FocusNode? _last;

  void remember(FocusNode? node) {
    if (node == null || !node.canRequestFocus) return;
    _last = node;
  }

  /// 优先拉回上次节点；否则 [fallback]（通常落到播停）。
  void restore({required VoidCallback fallback}) {
    final n = _last;
    if (n != null && n.canRequestFocus) {
      final ctx = n.context;
      if (ctx != null && ctx.mounted) {
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (n.canRequestFocus) n.requestFocus();
        });
        return;
      }
    }
    fallback();
  }

  void clear() => _last = null;
}
