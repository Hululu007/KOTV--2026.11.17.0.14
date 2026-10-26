import 'package:flutter/material.dart';

import 'kotv_android_surface.dart';

/// 原生 MPV Surface PlatformView。
Widget kotvMpvSurfaceView({
  Key? key,
  required String viewType,
  bool hybrid = true,
}) {
  return kotvAndroidSurface(
    key: key,
    viewType: viewType,
    creationParams: const <String, dynamic>{},
    hybrid: hybrid,
  );
}
