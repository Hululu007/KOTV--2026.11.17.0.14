import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

const _androidChannel = MethodChannel('kotv_android');

/// 清理 Android Exo SimpleCache（cacheDir/exo）。其它平台 no-op。
Future<void> kotvClearExoDiskCache() async {
  if (kIsWeb || defaultTargetPlatform != TargetPlatform.android) return;
  try {
    await _androidChannel.invokeMethod<void>('clearExoCache');
  } catch (_) {}
}

/// 安卓宿主缓存：cacheDir 下 jar/js/py/epg/jpa/mpv/proxy、codeCache 站点 jar；
/// [all] 时再删外置 `/TV/s_c_*`。其它平台 no-op。
Future<void> kotvClearAndroidCaches({
  required bool script,
  required bool jar,
  required bool other,
  required bool all,
}) async {
  if (kIsWeb || defaultTargetPlatform != TargetPlatform.android) return;
  try {
    await _androidChannel.invokeMethod<void>('clearCaches', <String, dynamic>{
      'script': script,
      'jar': jar,
      'other': other,
      'all': all,
    });
  } catch (_) {}
}
