import 'package:flutter/material.dart';

import 'kotv_android_surface.dart';

/// Exo 画面平台视图。
///
/// - [hybrid]=true：Hybrid Composition + SurfaceView（HDR 直出）
/// - [hybrid]=false：Virtual Display（兼容路径；HDR/10bit 可能异常）
Widget kotvExoSurfaceView({
  Key? key,
  required String viewType,
  required String fitName,
  required void Function(String fitName) onFit,
  bool hybrid = false,
}) {
  onFit(fitName);
  return kotvAndroidSurface(
    key: key,
    viewType: viewType,
    creationParams: <String, dynamic>{'fit': fitName},
    hybrid: hybrid,
  );
}
