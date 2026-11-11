import 'package:flutter/foundation.dart';

import 'kotv_platform.dart';
import 'player_normalize.dart';

/// FVP / libmdk 视频解码器列表（自动 / 硬解 / 软解）。
///
/// 这个列表必须交给 registerWith 的 `video.decoders`。插件 create 在 prepare
/// 之前读取它；initialize 之前调用 setVideoDecoders 没有 player，不会生效。
///
/// - **自动**：硬解优先 + FFmpeg/dav1d 回退
/// - **硬解**：仅平台硬解，不带软解回退
/// - **软解**：仅 FFmpeg（+ dav1d）
///
/// Android 不注册 FVP，此处无 AMediaCodec 分支。
List<String> kotvFvpVideoDecoders(String decodeMode) {
  final kind = kotvNormalizeDecodeMode(decodeMode);
  final soft = kind == 'soft';
  final hard = kind == 'hard';
  if (soft) return const ['FFmpeg', 'dav1d'];

  final withSoft = !hard;
  List<String> appendSoft(List<String> hw) =>
      withSoft ? [...hw, 'FFmpeg', 'dav1d'] : hw;

  if (!kIsWeb && defaultTargetPlatform == TargetPlatform.iOS) {
    return appendSoft(const ['VT']);
  }
  if (!kIsWeb && defaultTargetPlatform == TargetPlatform.macOS) {
    return appendSoft(const ['VT', 'hap']);
  }
  if (!kIsWeb && defaultTargetPlatform == TargetPlatform.linux) {
    return appendSoft(const ['VAAPI', 'CUDA', 'VDPAU', 'hap']);
  }
  if (!kIsWeb && defaultTargetPlatform == TargetPlatform.windows) {
    // Win7：先 DXVA（d3d9），再试不共享纹理的 MFT（mdk 文档标 win7+，shared/shader_resource
    // 仅 win8+）。FVP 的 D3D11 解码器没在 Win7 上验证过，不放进列表。fallback=0：失败必须返回，
    // 才能试下一个或落到自动模式的 FFmpeg/dav1d。
    if (kotvIsWindows7()) {
      return appendSoft(const [
        'DXVA',
        'MFT:d3d=11:shared=0:shader_resource=0:fallback=0',
      ]);
    }
    return appendSoft(const [
      'MFT:d3d=11',
      'D3D11',
      'DXVA',
      'CUDA',
      'hap',
    ]);
  }
  return appendSoft(const []);
}
