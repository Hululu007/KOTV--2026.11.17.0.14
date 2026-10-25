import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/kotv_api.dart';
import '../api/kotv_auth_token.dart';
import '../api/kotv_engine_url.dart';
import '../providers.dart';
import '../theme/kotv_palette.dart';
import 'chrome.dart';
import 'kotv_tv_text_field.dart';

/// Web 进页闸：同源本站必须先登录（不配远程后端；PC/安卓远端登录走设置页）。
Future<bool> ensureRemoteAuthIfNeeded(BuildContext context, WidgetRef ref) async {
  if (!kIsWeb) return true;
  final api = ref.read(apiProvider);

  final tok = await kotvAuthToken();
  if (tok.isNotEmpty) {
    try {
      await api.authMe();
      return true;
    } catch (_) {
      await kotvClearAuthToken();
    }
  }
  if (!context.mounted) return false;
  var allowReg = false;
  try {
    final st = await api.authStatus();
    allowReg = st['allowRegister'] == true;
  } catch (_) {}
  if (!context.mounted) return false;
  final r = await showRemoteLoginDialog(
    context,
    ref,
    api: api,
    allowRegister: allowReg,
    allowCancel: false,
    title: '登录',
    subtitle: '使用本站账号登录后即可观看',
  );
  return r.ok;
}

enum RemoteLoginPrompt {
  /// 本机、未开鉴权，或已有有效登录态。
  ready,
  /// 刚刚在弹窗里登录成功。
  signedIn,
  /// 用户取消或登录失败。
  cancelled,
  /// 引擎暂时连不上，交给原来的重试。
  skipped,
}

/// 远端地址需要登录时直接弹出登录框。
Future<RemoteLoginPrompt> promptRemoteLoginIfNeeded(
  BuildContext context,
  WidgetRef ref, {
  required String baseUrl,
  bool allowCancel = true,
}) async {
  final normalized = baseUrl.trim();
  if (normalized.isEmpty || kotvIsLocalEngineBaseUrl(normalized)) {
    return RemoteLoginPrompt.ready;
  }
  final probe = KotvApi(baseUrl: normalized);
  try {
    final h = await probe.health().timeout(const Duration(seconds: 6));
    if (h['ok'] != true) return RemoteLoginPrompt.skipped;
  } catch (_) {
    return RemoteLoginPrompt.skipped;
  }
  var allowReg = false;
  var needsLogin = false;
  try {
    final st = await probe.authStatus().timeout(const Duration(seconds: 4));
    needsLogin = st['remoteAccess'] == true || st['remoteAuth'] == true;
    allowReg = st['allowRegister'] == true;
  } catch (_) {}
  if (!needsLogin) return RemoteLoginPrompt.ready;
  final tok = await kotvAuthToken();
  if (tok.isNotEmpty) {
    try {
      await probe.authMe();
      return RemoteLoginPrompt.ready;
    } catch (_) {
      await kotvClearAuthToken();
    }
  }
  if (!context.mounted) return RemoteLoginPrompt.cancelled;
  final r = await showRemoteLoginDialog(
    context,
    ref,
    api: probe,
    allowRegister: allowReg,
    allowCancel: allowCancel,
    title: '远端登录',
    subtitle: '登录成功后才使用远端仓库',
  );
  return r.ok ? RemoteLoginPrompt.signedIn : RemoteLoginPrompt.cancelled;
}

bool kotvMessageNeedsLogin(Object error) {
  final s = '$error';
  return s.contains('需要登录') || s.contains('登录已失效');
}

/// 点播、直播、详情碰到未登录时直接弹登录框。
/// 先清掉失效 token，避免探测认为已经登录而把对话框跳过。
/// 登录成功返回 true。
Future<bool> kotvPresentLoginIfNeeded(BuildContext context, WidgetRef ref) async {
  if (!context.mounted) return false;
  await kotvClearAuthToken();
  final prompt = await promptRemoteLoginIfNeeded(
    context,
    ref,
    baseUrl: ref.read(engineLauncherProvider).baseUrl,
  );
  return prompt == RemoteLoginPrompt.signedIn;
}

class RemoteLoginResult {
  const RemoteLoginResult({required this.ok, this.username = ''});
  final bool ok;
  final String username;
}

/// [api] 可指向探测用临时客户端（尚未切换业务引擎时）。
Future<RemoteLoginResult> showRemoteLoginDialog(
  BuildContext context,
  WidgetRef ref, {
  KotvApi? api,
  bool allowRegister = false,
  bool allowCancel = true,
  String title = '远端登录',
  String? subtitle,
}) async {
  final KotvApi client = api ?? ref.read(apiProvider);
  final p = KotvPalette.of(context);
  final userCtrl = TextEditingController();
  final passCtrl = TextEditingController();
  var modeRegister = false;
  final hint = subtitle ??
      (kIsWeb ? '使用本站账号登录后即可观看' : '连接此引擎需登录一次，之后与本机使用相同');

  final ok = await showDialog<bool>(
    context: context,
    barrierDismissible: allowCancel,
    builder: (ctx) => StatefulBuilder(
      builder: (ctx, setLocal) => Dialog(
        backgroundColor: p.dialogBg,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(14)),
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 360),
          child: Padding(
            padding: const EdgeInsets.fromLTRB(20, 18, 20, 16),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Text(title, style: TextStyle(color: p.fg, fontWeight: FontWeight.w700, fontSize: 18)),
                const SizedBox(height: 8),
                Text(
                  modeRegister ? '创建账号后将自动登录' : hint,
                  style: TextStyle(color: p.muted, fontSize: 13),
                ),
                const SizedBox(height: 14),
                KotvTvTextField(
                  controller: userCtrl,
                  autofocus: true,
                  keyboardType: TextInputType.text,
                  textInputAction: TextInputAction.next,
                  style: TextStyle(color: p.fg),
                  decoration: InputDecoration(
                    hintText: '用户名',
                    hintStyle: TextStyle(color: p.muted),
                    filled: true,
                    fillColor: p.input,
                    border: OutlineInputBorder(borderRadius: BorderRadius.circular(10), borderSide: BorderSide.none),
                  ),
                ),
                const SizedBox(height: 10),
                KotvTvTextField(
                  controller: passCtrl,
                  obscureText: true,
                  keyboardType: TextInputType.visiblePassword,
                  textInputAction: TextInputAction.done,
                  onSubmitted: (_) => Navigator.pop(ctx, true),
                  style: TextStyle(color: p.fg),
                  decoration: InputDecoration(
                    hintText: '密码',
                    hintStyle: TextStyle(color: p.muted),
                    filled: true,
                    fillColor: p.input,
                    border: OutlineInputBorder(borderRadius: BorderRadius.circular(10), borderSide: BorderSide.none),
                  ),
                ),
                if (allowRegister) ...[
                  const SizedBox(height: 8),
                  Align(
                    alignment: Alignment.centerLeft,
                    child: KotvFocusAction(
                      label: modeRegister ? '已有账号？去登录' : '没有账号？注册',
                      color: p.primary,
                      onPressed: () => setLocal(() => modeRegister = !modeRegister),
                    ),
                  ),
                ],
                const SizedBox(height: 12),
                Row(
                  children: [
                    if (allowCancel) ...[
                      Expanded(
                        child: KotvFocusAction(
                          label: '取消',
                          color: p.muted,
                          onPressed: () => Navigator.pop(ctx, false),
                        ),
                      ),
                      const SizedBox(width: 8),
                    ],
                    Expanded(
                      child: KotvFocusAction(
                        label: modeRegister ? '注册并登录' : '登录',
                        filled: true,
                        onPressed: () => Navigator.pop(ctx, true),
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
    ),
  );
  if (ok != true) return const RemoteLoginResult(ok: false);
  final u = userCtrl.text.trim();
  final pw = passCtrl.text;
  if (u.isEmpty || pw.isEmpty) return const RemoteLoginResult(ok: false);
  try {
    if (modeRegister) {
      await client.register(u, pw);
    }
    await client.login(u, pw);
    ref.invalidate(configProvider);
    ref.invalidate(settingsProvider);
    return RemoteLoginResult(ok: true, username: u);
  } catch (e) {
    if (context.mounted) {
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text('$e')));
    }
    return const RemoteLoginResult(ok: false);
  }
}
