// 字幕 / 弹幕样式设置值归一。

import 'kotv_playback.dart';
import 'player_normalize.dart';
import 'video_eq.dart' show kotvSubtitlePosFromSettings;

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

/// 按设置写当前后端的字幕样式与时间偏移（点播 / 直播共用）。
/// 「原样」不覆盖片源样式；「自定义」写颜色/描边/背景；「系统」只写透明度与字体。
Future<void> kotvApplySubtitleSettings(KotvPlayback p, Map<String, dynamic> settings) async {
  String str(String k, String def) => '${settings[k] ?? def}'.trim();
  double num(String k, double def) => double.tryParse(str(k, '$def')) ?? def;
  final styleMode = str('subtitleStyleMode', 'original').toLowerCase();
  final custom = styleMode == 'custom';
  final system = styleMode == 'system';
  final styled = custom || system;
  final color = str('subtitleColor', '#FFFFFF');
  final border = str('subtitleBorderColor', '#000000');
  final bg = str('subtitleBgColor', '#00000000');
  final fontPath = str('subtitleFontPath', '');
  await p.setSubtitleStyle(
    scale: kotvClampSubtitleFontScale(num('subtitleFontScale', 1.0)),
    pos: kotvSubtitlePosFromSettings(settings['subtitlePos']).clamp(-20, 30).toDouble(),
    secondaryPos: num('subtitleSecondaryPos', 10).clamp(0, 150).toDouble(),
    color: custom && color.isNotEmpty ? color : null,
    borderColor: custom && border.isNotEmpty ? border : null,
    borderSize: custom ? num('subtitleBorderSize', 2).clamp(0, 8).toDouble() : null,
    bgColor: custom && bg.isNotEmpty ? bg : null,
    edgeType: custom ? str('subtitleEdgeType', 'outline').toLowerCase() : null,
    useSystemStyle: system,
    textOpacity: styled ? num('subtitleTextOpacity', 100).clamp(0, 100).toDouble() : null,
    bgOpacity: styled ? num('subtitleBgOpacity', 100).clamp(0, 100).toDouble() : null,
    edgeOpacity: styled ? num('subtitleEdgeOpacity', 100).clamp(0, 100).toDouble() : null,
    shadowStrength: styled ? kotvSubtitleShadowStrength(str('subtitleShadowStrength', '50')) : null,
    font: styled ? kotvNormalizeSubtitleFont(str('subtitleFont', 'default')) : null,
    fontPath: styled && fontPath.isNotEmpty ? fontPath : null,
    forceStyle: custom,
  );
  final offset = int.tryParse(str('subtitleOffsetMs', '0')) ?? 0;
  await p.setSubtitleOffsetMs(offset.clamp(-300000, 300000));
}
