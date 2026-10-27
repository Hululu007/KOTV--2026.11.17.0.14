import '../util/kotv_io.dart';

import 'package:flutter/cupertino.dart';
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';

/// 详情页路由：移动端用 [CupertinoPageRoute] 以支持边缘侧滑返回。
Route<T> kotvDetailRoute<T>({required WidgetBuilder builder}) {
  if (!kIsWeb && (Platform.isIOS || Platform.isAndroid)) {
    return CupertinoPageRoute<T>(builder: builder);
  }
  return MaterialPageRoute<T>(builder: builder);
}

/// 主 Tab 换页：短淡入，避免 Material 侧滑 + 整页重建显得拖沓。
Route<T> kotvTabRoute<T>({
  required WidgetBuilder builder,
  RouteSettings? settings,
}) {
  return PageRouteBuilder<T>(
    settings: settings,
    pageBuilder: (context, animation, secondaryAnimation) => builder(context),
    transitionDuration: const Duration(milliseconds: 160),
    reverseTransitionDuration: const Duration(milliseconds: 120),
    transitionsBuilder: (context, animation, secondaryAnimation, child) {
      return FadeTransition(
        opacity: CurvedAnimation(parent: animation, curve: Curves.easeOutCubic),
        child: child,
      );
    },
  );
}
