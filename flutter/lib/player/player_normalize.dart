// 各内置播放器共用的归一 / clamp，避免 exo / fvp / mpv / failover 各写一份。

/// soft / hard / auto（含 software、hw 等别名）。
String kotvNormalizeDecodeMode(String raw) {
  switch (raw.trim().toLowerCase()) {
    case 'soft':
    case 'software':
    case 'sw':
      return 'soft';
    case 'hard':
    case 'hardware':
    case 'hw':
      return 'hard';
    default:
      return 'auto';
  }
}

double kotvClampSubtitleFontScale(num? raw, [double fallback = 1.0]) {
  final v = raw?.toDouble() ?? fallback;
  return v.clamp(0.5, 2.0);
}

double kotvClampPlayRate(num? raw, [double fallback = 1.0]) {
  final v = raw?.toDouble() ?? fallback;
  return v.clamp(0.25, 4.0);
}
