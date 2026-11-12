import 'package:flutter_test/flutter_test.dart';

import 'package:kotv/player/playback_failover.dart';

void main() {
  test('hard falls back to auto then next player restores settings decode', () {
    final f = KotvPlaybackFailover(playerVal: 'innie#fvp', decodeMode: 'hard');
    f.markAttempt();
    final step = f.nextStep();
    expect(step, isNotNull);
    expect(step!.kind, KotvFailoverKind.autoDecode);
    expect(step.decodeMode, 'auto');
    expect(f.decodeMode, 'auto');

    final next = f.nextStep();
    expect(next, isNotNull);
    expect(next!.kind, KotvFailoverKind.nextPlayer);
    expect(next.playerVal, 'innie#mpv');
    expect(next.decodeMode, 'hard');
  });

  test('mpv hard falls back to auto like other engines', () {
    final f = KotvPlaybackFailover(playerVal: 'innie#mpv', decodeMode: 'hard');
    f.markAttempt();
    final step = f.nextStep();
    expect(step, isNotNull);
    expect(step!.kind, KotvFailoverKind.autoDecode);
    expect(step.decodeMode, 'auto');
    expect(f.decodeMode, 'auto');
  });

  test('auto goes straight to the next player', () {
    final f = KotvPlaybackFailover(playerVal: 'innie#mpv', decodeMode: 'auto');
    f.markAttempt();
    final next = f.nextStep();
    expect(next, isNotNull);
    expect(next!.kind, KotvFailoverKind.nextPlayer);
    expect(next.playerVal, 'innie#fvp');
    expect(next.decodeMode, 'auto');
  });

  test('drm lock gives up', () {
    final f = KotvPlaybackFailover(
      playerVal: 'innie#exo',
      decodeMode: 'auto',
      lockExoForDrm: true,
    );
    f.markAttempt();
    expect(f.nextStep(), isNull);
  });

  test('soft goes straight to the next player', () {
    final f = KotvPlaybackFailover(playerVal: 'innie#fvp', decodeMode: 'soft');
    f.markAttempt();
    final next = f.nextStep();
    expect(next!.playerVal, 'innie#mpv');
    expect(next.decodeMode, 'soft');
  });

  test('disabled failover gives up', () {
    final f = KotvPlaybackFailover(
      playerVal: 'innie#mpv',
      decodeMode: 'hard',
      enabled: false,
    );
    f.markAttempt();
    expect(f.nextStep(), isNull);
  });

  test('enabledFromSetting', () {
    expect(KotvPlaybackFailover.enabledFromSetting('auto'), isTrue);
    expect(KotvPlaybackFailover.enabledFromSetting(''), isTrue);
    expect(KotvPlaybackFailover.enabledFromSetting('off'), isFalse);
    expect(KotvPlaybackFailover.enabledFromSetting('false'), isFalse);
  });

  test('off does not flip decode or change player', () {
    final f = KotvPlaybackFailover(
      playerVal: 'innie#mpv',
      decodeMode: 'hard',
      enabled: false,
    );
    f.markAttempt();
    expect(f.nextStep(), isNull);
    expect(f.playerVal, 'innie#mpv');
    expect(f.decodeMode, 'hard');
  });

  test('plan: off + desktop mpv retries same engine', () {
    final f = KotvPlaybackFailover(
      playerVal: 'innie#mpv',
      decodeMode: 'auto',
      enabled: false,
    );
    f.markAttempt();
    final plan = kotvPlanSilentRecover(failover: f, canRecreateDesktopMpv: true);
    expect(plan.action, KotvSilentRecoverAction.retrySameEngine);
    expect(f.playerVal, 'innie#mpv');
  });

  test('plan: off + other engine keeps session', () {
    final f = KotvPlaybackFailover(
      playerVal: 'innie#fvp',
      decodeMode: 'auto',
      enabled: false,
    );
    f.markAttempt();
    final plan = kotvPlanSilentRecover(failover: f, canRecreateDesktopMpv: false);
    expect(plan.action, KotvSilentRecoverAction.keepSession);
  });

  test('plan: on + next player applies step', () {
    final f = KotvPlaybackFailover(playerVal: 'innie#mpv', decodeMode: 'auto');
    f.markAttempt();
    final plan = kotvPlanSilentRecover(failover: f, canRecreateDesktopMpv: true);
    expect(plan.action, KotvSilentRecoverAction.applyStep);
    expect(plan.step!.playerVal, 'innie#fvp');
    expect(f.playerVal, 'innie#fvp');
  });

  test('plan: on + exhausted ring recreates desktop mpv', () {
    final f = KotvPlaybackFailover(playerVal: 'innie#mpv', decodeMode: 'auto');
    f.markAttempt();
    // 先耗尽环上其它引擎。
    while (true) {
      final s = f.nextStep();
      if (s == null) break;
      f.markAttempt();
    }
    final plan = kotvPlanSilentRecover(failover: f, canRecreateDesktopMpv: true);
    expect(plan.action, KotvSilentRecoverAction.retrySameEngine);
  });

  test('plan: on + exhausted ring without recreate gives up', () {
    final f = KotvPlaybackFailover(playerVal: 'innie#fvp', decodeMode: 'auto');
    f.markAttempt();
    while (true) {
      final s = f.nextStep();
      if (s == null) break;
      f.markAttempt();
    }
    final plan = kotvPlanSilentRecover(failover: f, canRecreateDesktopMpv: false);
    expect(plan.action, KotvSilentRecoverAction.giveUp);
  });
}
