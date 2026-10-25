import 'package:flutter/foundation.dart' show defaultTargetPlatform, kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/kotv_api.dart';
import '../api/kotv_auth_token.dart';
import '../api/kotv_client_store.dart';
import '../api/kotv_engine_url.dart';
import '../api/kotv_settings_cache.dart';
import '../providers.dart';
import '../theme/kotv_palette.dart';
import '../theme/layout_scale.dart';
import '../util/kotv_image_cache.dart';
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
  bool assumeReachable = false,
}) async {
  final normalized = baseUrl.trim();
  if (normalized.isEmpty || kotvIsLocalEngineBaseUrl(normalized)) {
    return RemoteLoginPrompt.ready;
  }
  final probe = KotvApi(baseUrl: normalized);
  if (!assumeReachable) {
    try {
      final h = await probe.health().timeout(const Duration(seconds: 6));
      if (h['ok'] != true) return RemoteLoginPrompt.skipped;
    } catch (_) {
      return RemoteLoginPrompt.skipped;
    }
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
  if (!context.mounted) return false;
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

/// 已保存的远端地址。本机地址不算。
Future<String> kotvSavedRemoteEngineAddress(WidgetRef ref) async {
  final live = ref.read(engineLauncherProvider).baseUrl.trim();
  if (live.isNotEmpty && !kotvIsLocalEngineBaseUrl(live)) return live;
  final data = await KotvClientStore.load();
  for (final raw in [data.engineBaseUrl, data.engineBaseUrlDraft]) {
    final trimmed = raw.trim();
    if (trimmed.isEmpty || kotvIsLocalEngineBaseUrl(trimmed)) continue;
    final effective = kotvEffectiveEngineBaseUrl(trimmed);
    if (!kotvIsLocalEngineBaseUrl(effective)) return effective;
  }
  return '';
}

Future<void> kotvUseLocalEngine(WidgetRef ref) async {
  await KotvClientStore.clearEngine();
  ref.read(engineLauncherProvider).applyBaseUrl('');
  ref.invalidate(engineReadyProvider);
}

/// 地址可用后写入并登录。用户取消登录则保留地址，不进入首页。
Future<void> kotvFinishEngineAddress(BuildContext context, WidgetRef ref, String url) async {
  await KotvClientStore.setEngineBaseUrl(url);
  ref.read(engineLauncherProvider).applyBaseUrl(url);
  KotvSettingsCache.clear();
  if (context.mounted) {
    await kotvClearImageMemory(ref: ref);
  }
  if (!context.mounted) return;
  final prompt = await promptRemoteLoginIfNeeded(
    context,
    ref,
    baseUrl: url,
    assumeReachable: true,
  );
  if (!context.mounted || prompt == RemoteLoginPrompt.cancelled) return;
  ref.invalidate(engineReadyProvider);
}

/// 填写或改正后端地址。探测失败时弹窗说明，输入框保持打开。
/// 取消返回 null。非 iOS 清空地址时返回空串，表示改回本机。
Future<String?> kotvPromptEngineAddress(
  BuildContext context, {
  String initial = '',
  String errorText = '',
}) async {
  final p = KotvPalette.of(context);
  final ctrl = TextEditingController(text: initial);
  final ios = !kIsWeb && defaultTargetPlatform == TargetPlatform.iOS;
  var busy = false;
  final result = await showDialog<String>(
    context: context,
    builder: (ctx) => StatefulBuilder(
      builder: (ctx, setLocal) {
        Future<void> submit() async {
          if (busy) return;
          final raw = ctrl.text.trim();
          final stored = kotvStoreEngineBaseUrl(raw);
          if (stored.isEmpty) {
            if (ios) {
              showAppNews(ctx, raw.isEmpty ? '请填写后端地址' : '没有本机引擎，请填写远端地址');
              return;
            }
            if (ctx.mounted) Navigator.pop(ctx, '');
            return;
          }
          setLocal(() => busy = true);
          final probe = KotvApi(baseUrl: stored);
          try {
            final h = await probe.health().timeout(const Duration(seconds: 6));
            if (h['ok'] != true) {
              throw Exception('${h['error'] ?? '引擎未就绪'}');
            }
          } catch (e) {
            if (ctx.mounted) {
              setLocal(() => busy = false);
              showAppNews(ctx, '无法连接\n$stored\n$e');
            }
            return;
          }
          if (ctx.mounted) Navigator.pop(ctx, stored);
        }

        return AlertDialog(
          backgroundColor: p.dialogBg,
          title: Text(
            errorText.isEmpty ? '连接后端服务' : '无法连接',
            style: TextStyle(color: p.fg, fontWeight: FontWeight.w700),
          ),
          content: SizedBox(
            width: LayoutScale.dp(ctx, 520),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                if (errorText.isNotEmpty) ...[
                  Text(errorText, style: TextStyle(color: p.muted)),
                  const SizedBox(height: 12),
                ],
                KotvTvTextField(
                  controller: ctrl,
                  autofocus: true,
                  style: TextStyle(color: p.fg),
                  decoration: InputDecoration(
                    hintText: 'http://10.0.0.8:9978 或 https://api.example.com',
                    hintStyle: TextStyle(color: p.muted),
                  ),
                  onSubmitted: (_) => submit(),
                ),
              ],
            ),
          ),
          actions: [
            KotvFocusAction(label: '取消', color: p.muted, onPressed: () => Navigator.pop(ctx)),
            KotvFocusAction(label: '保存并重试', filled: true, onPressed: submit),
          ],
        );
      },
    ),
  );
  return result;
}

/// 重试当前地址。连不上就弹出地址框；能连上且要登录就弹出登录框。
Future<void> kotvRetryEngineConnect(BuildContext context, WidgetRef ref) async {
  final saved = await kotvSavedRemoteEngineAddress(ref);
  if (!context.mounted) return;
  if (saved.isEmpty) {
    final typed = await kotvPromptEngineAddress(context);
    if (typed == null || !context.mounted) return;
    if (typed.isEmpty) {
      await kotvUseLocalEngine(ref);
      return;
    }
    await kotvFinishEngineAddress(context, ref, typed);
    return;
  }
  var reachable = false;
  Object? failure;
  try {
    final h = await KotvApi(baseUrl: saved).health().timeout(const Duration(seconds: 6));
    reachable = h['ok'] == true;
    if (!reachable) failure = h['error'] ?? '引擎未就绪';
  } catch (e) {
    failure = e;
  }
  if (!context.mounted) return;
  if (!reachable) {
    final typed = await kotvPromptEngineAddress(
      context,
      initial: saved,
      errorText: '无法连接 $saved${failure == null ? '' : '\n$failure'}',
    );
    if (typed == null || !context.mounted) return;
    if (typed.isEmpty) {
      await kotvUseLocalEngine(ref);
      return;
    }
    await kotvFinishEngineAddress(context, ref, typed);
    return;
  }
  await kotvFinishEngineAddress(context, ref, saved);
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

  var busy = false;
  final ok = await showDialog<bool>(
    context: context,
    barrierDismissible: allowCancel,
    builder: (ctx) => StatefulBuilder(
      builder: (ctx, setLocal) {
        Future<void> submit() async {
          if (busy) return;
          final u = userCtrl.text.trim();
          final pw = passCtrl.text;
          if (u.isEmpty || pw.isEmpty) {
            showAppNews(ctx, '请填写用户名和密码');
            return;
          }
          setLocal(() => busy = true);
          try {
            if (modeRegister) {
              await client.register(u, pw);
            }
            await client.login(u, pw);
            if (ctx.mounted) Navigator.pop(ctx, true);
          } catch (e) {
            if (ctx.mounted) showAppNews(ctx, '$e');
          } finally {
            if (ctx.mounted) setLocal(() => busy = false);
          }
        }

        return Dialog(
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
                  onSubmitted: (_) => submit(),
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
                        onPressed: submit,
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
        ),
        );
      },
    ),
  );
  if (ok != true) return const RemoteLoginResult(ok: false);
  ref.invalidate(configProvider);
  ref.invalidate(settingsProvider);
  return RemoteLoginResult(ok: true, username: userCtrl.text.trim());
}
