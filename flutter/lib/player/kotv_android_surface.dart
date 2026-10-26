import 'package:flutter/foundation.dart';
import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter/services.dart';

/// Android PlatformView：Hybrid Composition 或 Virtual Display。
Widget kotvAndroidSurface({
  Key? key,
  required String viewType,
  required Map<String, dynamic> creationParams,
  required bool hybrid,
}) {
  if (hybrid) {
    return PlatformViewLink(
      key: key,
      viewType: viewType,
      surfaceFactory: (context, controller) {
        return AndroidViewSurface(
          controller: controller as AndroidViewController,
          hitTestBehavior: PlatformViewHitTestBehavior.transparent,
          gestureRecognizers: const <Factory<OneSequenceGestureRecognizer>>{},
        );
      },
      onCreatePlatformView: (params) {
        final controller = PlatformViewsService.initExpensiveAndroidView(
          id: params.id,
          viewType: viewType,
          layoutDirection: TextDirection.ltr,
          creationParams: creationParams,
          creationParamsCodec: const StandardMessageCodec(),
        );
        controller.addOnPlatformViewCreatedListener(params.onPlatformViewCreated);
        controller.create();
        return controller;
      },
    );
  }
  return AndroidView(
    key: key,
    viewType: viewType,
    layoutDirection: TextDirection.ltr,
    creationParams: creationParams,
    creationParamsCodec: const StandardMessageCodec(),
    hitTestBehavior: PlatformViewHitTestBehavior.transparent,
  );
}
