/// 跨 Exo / MPV / FVP 共用的播放设置键读取。
/// 键名历史为 exo*（设置白名单与落盘已固定），各引擎均读同一键。
int kotvDolbyVisionFromSettings(Map<String, dynamic> settings) {
  return int.tryParse('${settings['exoDolbyVision'] ?? '0'}') ?? 0;
}

/// 双字幕 `exoSecondarySubtitle`：off / auto（另一语言）/ default（跟随播放器）/ manual。
String kotvSecondarySubtitleModeFromSettings(Map<String, dynamic> settings) {
  return switch ('${settings['exoSecondarySubtitle'] ?? 'default'}'.trim().toLowerCase()) {
    'off' || 'no' || 'false' || '0' || '' => 'off',
    'auto' || 'on' => 'auto',
    'manual' => 'manual',
    _ => 'default',
  };
}

String kotvPreferredTextLangsFromSettings(Map<String, dynamic> settings) {
  return '${settings['exoPreferredTextLangs'] ?? ''}'.trim();
}
