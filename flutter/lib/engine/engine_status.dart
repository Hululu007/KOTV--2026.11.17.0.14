import 'package:flutter/foundation.dart' show kIsWeb;

import '../api/kotv_api.dart';
import '../api/kotv_auth_token.dart';
import '../api/kotv_engine_url.dart';
import 'engine_launcher.dart';

enum KotvEngineState {
  /// 可以进入应用。
  ready,

  /// 远端后端可用，但要先登录。
  signIn,

  /// 连不上，或不是可用的 KOTV 后端。
  offline,
}

/// 进入应用前的引擎状态。
class KotvEngineStatus {
  const KotvEngineStatus(this.state, {this.error = '', this.allowRegister = false});

  final KotvEngineState state;

  /// [KotvEngineState.offline] 的原因。
  final String error;

  /// 远端是否开放注册（登录框显示「注册」）。
  final bool allowRegister;

  bool get ready => state == KotvEngineState.ready;
}

/// 本机地址免登录；远端地址和 Web 页面必须登录后才能使用。
bool kotvEngineNeedsSignIn(String baseUrl) => kIsWeb || !kotvIsLocalEngineBaseUrl(baseUrl);

/// 检查当前引擎：本机连上即可；远端须是同协议、身份未变、开放了远端访问的 KOTV 后端，
/// 且记住的登录态仍有效。
Future<KotvEngineStatus> kotvCheckEngine(EngineLauncher launcher) async {
  if (!await launcher.recoverIfNeeded()) {
    return KotvEngineStatus(KotvEngineState.offline, error: launcher.lastError);
  }
  if (!kotvEngineNeedsSignIn(launcher.baseUrl)) {
    return const KotvEngineStatus(KotvEngineState.ready);
  }
  final api = launcher.client();
  try {
    final identity = await api.engineIdentity();
    final st = await api.authStatus();
    if (st['remoteAccess'] != true) {
      return const KotvEngineStatus(
        KotvEngineState.offline,
        error: '对方未开放远端访问，请在主机「用户管理」中打开远端鉴权',
      );
    }
    if ((await kotvAuthToken()).isNotEmpty) {
      try {
        await api.authMe();
        await api.rememberEngineIdentity(identity);
        return const KotvEngineStatus(KotvEngineState.ready);
      } on KotvApiException {
        await kotvClearAuthToken();
      }
    }
    return KotvEngineStatus(KotvEngineState.signIn, allowRegister: st['allowRegister'] == true);
  } catch (e) {
    return KotvEngineStatus(KotvEngineState.offline, error: '$e');
  }
}
