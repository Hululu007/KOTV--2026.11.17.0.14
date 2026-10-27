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

/// 主 Tab 换页。
/// 桌面：Material 默认转场；安卓：无动画（整页重建时淡入更卡）；其它：短淡入。
Route<T> kotvTabRoute<T>({
  required WidgetBuilder builder,
  RouteSettings? settings,
}) {
  if (!kIsWeb &&
      (Platform.isMacOS || Platform.isWindows || Platform.isLinux)) {
    return MaterialPageRoute<T>(settings: settings, builder: builder);
  }
  if (!kIsWeb && Platform.isAndroid) {
    return PageRouteBuilder<T>(
      settings: settings,
      pageBuilder: (context, animation, secondaryAnimation) =>
          builder(context),
      transitionDuration: Duration.zero,
      reverseTransitionDuration: Duration.zero,
    );
  }
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
