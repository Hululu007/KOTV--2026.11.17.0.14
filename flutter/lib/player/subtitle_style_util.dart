// 字幕 / 弹幕样式设置值归一。

String kotvNormalizeSubtitleEdgeType(String? raw) {
  final e = (raw ?? '').trim().toLowerCase();
  return switch (e) {
    'none' || 'shadow' || 'raised' || 'depressed' => e,
    _ => 'outline',
  };
}

/// default / sans / serif / mono。
String kotvNormalizeSubtitleFont(String? raw) {
  final f = (raw ?? '').trim().toLowerCase();
  return switch (f) {
    'sans' || 'sans-serif' || 'serif' || 'mono' || 'monospace' => f == 'sans-serif'
        ? 'sans'
        : (f == 'monospace' ? 'mono' : f),
    _ => 'default',
  };
}

/// 0–100；默认 50。
double kotvSubtitleShadowStrength(String? raw, {double def = 50}) {
  return (double.tryParse('${raw ?? ''}') ?? def).clamp(0, 100);
}

/// none / outline / shadow / projection。空或未知时默认 outline（硬描边，减轻拖影）。
String kotvNormalizeDanmakuStroke(String? raw) {
  final s = (raw ?? '').trim().toLowerCase();
  return switch (s) {
    'none' || 'outline' || 'shadow' || 'projection' => s,
    _ => 'outline',
  };
}

/// original / white / yellow。
String kotvNormalizeDanmakuColorMode(String? raw) {
  final s = (raw ?? '').trim().toLowerCase();
  return switch (s) {
    'white' || 'yellow' => s,
    _ => 'original',
  };
}
