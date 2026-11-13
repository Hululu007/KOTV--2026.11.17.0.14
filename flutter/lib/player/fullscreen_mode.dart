import 'dart:math' as math;

import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:window_manager/window_manager.dart';

import '../theme/layout_scale.dart';
import 'fullscreen_sys.dart' if (dart.library.html) 'fullscreen_sys_web.dart' as sys;
import 'kotv_playback.dart';
import 'kotv_platform.dart';

/// 桌面/Web 全屏：铺满当前窗口，或占满整块屏幕。
enum KotvDesktopFullscreenKind {
  /// 仅进入应用内全屏页/沉浸布局，不改系统窗口 / 浏览器全屏。
  window,

  /// 系统全屏（桌面）或浏览器全屏（Web），占满整块显示器。
  display,
}

/// 沉浸布局已 setState 后调用：先等这一帧画完，再进系统全屏。
/// 详情 / 直播共用，避免各页各自写 endOfFrame + enter 时序漂移。
Future<void> kotvEnterSystemFullscreenAfterLayout(
  KotvDesktopFullscreenKind kind,
) async {
  await WidgetsBinding.instance.endOfFrame;
  await kotvEnterSystemFullscreen(kind);
}

Future<void> kotvEnterSystemFullscreen(KotvDesktopFullscreenKind kind) async {
  try {
    await SystemChrome.setEnabledSystemUIMode(SystemUiMode.immersiveSticky);
  } catch (_) {}
  try {
    // 沉浸时去掉状态栏/导航栏对比色占位，避免仍像「顶上一条」。
    SystemChrome.setSystemUIOverlayStyle(
      const SystemUiOverlayStyle(
        statusBarColor: Colors.transparent,
        systemNavigationBarColor: Colors.transparent,
        statusBarIconBrightness: Brightness.light,
        systemNavigationBarIconBrightness: Brightness.light,
      ),
    );
  } catch (_) {}
  if (kind != KotvDesktopFullscreenKind.display) return;
  if (kIsWeb) {
    await sys.kotvEnterDisplayFullscreen();
    return;
  }
  if (!kotvIsDesktop()) return;
  // Windows：先等应用内布局落稳再 setFullScreen，避免和画面放大挤在同一帧。
  // 有边框不先最大化。子窗对齐客户区由 window_manager 补丁延后按条件重试。
  if (kotvIsWindows()) {
    try {
      if (await windowManager.isFullScreen()) return;
    } catch (_) {}
    await Future<void>.delayed(const Duration(milliseconds: 180));
    try {
      await WidgetsBinding.instance.endOfFrame;
    } catch (_) {}
  }
  try {
    await windowManager.setFullScreen(true);
  } catch (_) {}
}

const _phoneOrientations = <DeviceOrientation>[
  DeviceOrientation.portraitUp,
  DeviceOrientation.landscapeLeft,
  DeviceOrientation.landscapeRight,
];

/// 手机恢复可横可竖。全屏里临时锁过方向后必须走这里，否则回首页也转不了。
Future<void> kotvRestorePhoneOrientations() async {
  if (kIsWeb || kotvIsDesktop() || KotvLayout.television) return;
  try {
    await SystemChrome.setPreferredOrientations(_phoneOrientations);
  } catch (_) {}
}

Future<void> kotvExitSystemFullscreen({required bool wasDisplayFullscreen}) async {
  try {
    await SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
  } catch (_) {}
  try {
    if (KotvLayout.television) {
      await SystemChrome.setPreferredOrientations(const [
        DeviceOrientation.landscapeLeft,
        DeviceOrientation.landscapeRight,
      ]);
    } else if (!kIsWeb && !kotvIsDesktop()) {
      await kotvRestorePhoneOrientations();
    } else {
      await SystemChrome.setPreferredOrientations(DeviceOrientation.values);
    }
  } catch (_) {}
  if (!wasDisplayFullscreen) return;
  if (kIsWeb) {
    await sys.kotvExitDisplayFullscreen();
    return;
  }
  if (kotvIsDesktop()) {
    try {
      await windowManager.setFullScreen(false);
    } catch (_) {}
  }
}

Future<void> kotvLockPortrait() async {
  try {
    await SystemChrome.setPreferredOrientations(const [
      DeviceOrientation.portraitUp,
    ]);
  } catch (_) {}
}

Future<void> kotvForceLandscape() async {
  try {
    await SystemChrome.setPreferredOrientations(const [
      DeviceOrientation.landscapeLeft,
      DeviceOrientation.landscapeRight,
    ]);
  } catch (_) {}
}

/// 仅「手机竖屏握持 + 横屏片源」时显示「全屏观看」。
/// 竖屏片、方片、尺寸未出（加载中）都不显示。
bool kotvShouldShowForceLandscape({
  required Size screen,
  required int videoWidth,
  required int videoHeight,
}) {
  if (kotvIsDesktop()) return false;
  final phonePortrait = screen.height > screen.width;
  if (!phonePortrait) return false;
  if (videoWidth <= 0 || videoHeight <= 0) return false;
  return videoWidth > videoHeight;
}

/// BoxFit.contain 下视频实际绘制区域（用于把控件放到 letterbox 黑边）。
Rect kotvVideoContainRect({
  required Size screen,
  required int videoWidth,
  required int videoHeight,
}) {
  var vw = videoWidth.toDouble();
  var vh = videoHeight.toDouble();
  if (vw <= 0 || vh <= 0) {
    vw = 16;
    vh = 9;
  }
  final scale = math.min(screen.width / vw, screen.height / vh);
  final dw = vw * scale;
  final dh = vh * scale;
  return Rect.fromLTWH(
    (screen.width - dw) / 2,
    (screen.height - dh) / 2,
    dw,
    dh,
  );
}

/// 旧 Android / Rockchip HWC：退出沉浸改图层时若仍挂着
/// SurfaceView，`SquashFrame`/`PrepareAndCacheFramebuffer` 易对空
/// GraphicBuffer SIGSEGV，拖垮 surfaceflinger 整机重启。
/// 电视盒或原生判定需 park 时，退出前先卸画面输出。
bool? _hwcSurfaceParkCached;

Future<bool> kotvShouldParkSurfaceForImmersiveExit() async {
  if (!kotvIsAndroid()) return false;
  if (KotvLayout.television) return true;
  final cached = _hwcSurfaceParkCached;
  if (cached != null) return cached;
  try {
    final v = await const MethodChannel('kotv_android')
        .invokeMethod<bool>('needsHwcSurfacePark');
    return _hwcSurfaceParkCached = v == true;
  } catch (_) {
    return _hwcSurfaceParkCached = false;
  }
}

Future<void> _awaitFrames(int n) async {
  for (var i = 0; i < n; i++) {
    try {
      await WidgetsBinding.instance.endOfFrame;
    } catch (_) {}
  }
}

/// 卸掉视频 Surface（及 OSD 挖空层），再等两帧让合成器消化。
Future<void> kotvParkSurfaceForImmersiveExit(KotvPlayback playback) async {
  try {
    await playback.setVideoOutputEnabled(false);
  } catch (_) {}
  await _awaitFrames(2);
}

/// 几何已缩回详情/列表槽后再调用：挂回画面并等一帧 rebuild。
/// 缩槽前 unpark 会使新建 SurfaceView 随 Positioned 立刻 surfaceDestroyed。
Future<void> kotvUnparkSurfaceAfterImmersiveExit(KotvPlayback playback) async {
  try {
    await playback.setVideoOutputEnabled(true);
  } catch (_) {}
  await _awaitFrames(1);
}

/// 详情 / 直播退沉浸唯一入口：需 HWC park 时先卸面，[exitBody] 改图层后再挂回。
Future<void> kotvRunImmersiveExitGuardingHwcSurface({
  required KotvPlayback playback,
  required Future<void> Function() exitBody,
}) async {
  final park = await kotvShouldParkSurfaceForImmersiveExit();
  if (park) await kotvParkSurfaceForImmersiveExit(playback);
  try {
    await exitBody();
  } finally {
    if (park) await kotvUnparkSurfaceAfterImmersiveExit(playback);
  }
}
