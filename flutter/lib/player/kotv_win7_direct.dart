import 'mpv_opts.dart';

/// Win7「硬解直出」切换：落盘值 + 是否须换 Player（ANGLE 后端随 dxva2 变）。
/// 详情 / 直播 / 设置共用，避免各写一份。
String kotvWin7DirectNextValue(bool currentlyOn) => currentlyOn ? 'false' : 'true';

bool kotvWin7DirectToggleNeedsNewPlayer({
  required KotvMpvOpts from,
  required bool nextDirect,
}) =>
    kotvMediaKitNeedsNewPlayer(from, from.copyWith(direct: nextDirect));
