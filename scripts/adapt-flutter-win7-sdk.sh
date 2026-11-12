#!/usr/bin/env bash
# 将 Flutter 工程约束临时放宽到 3.19 / Dart 3.3，供 Win7 实验线 CI / 本地 Win7 Flutter 使用。
# 不提交改动到 git；仅影响当前工作区。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APP="$ROOT/flutter/pubspec.yaml"

[[ -f "$APP" ]] || { echo "missing $APP" >&2; exit 1; }

# 主工程：Dart 3.3（Flutter 3.19）
perl -i -pe 's/sdk:\s*\^3\.[0-9.]+/sdk: ">=3.3.0 <3.4.0"/' "$APP"
perl -i -pe 's/flutter_lints:\s*\^4\.0\.0/flutter_lints: ^3.0.0/' "$APP"
perl -i -pe 's/path_provider:\s*\^2\.1\.5/path_provider: 2.1.4/' "$APP"
perl -i -pe 's/shared_preferences:\s*\^2\.3\.3/shared_preferences: 2.3.0/' "$APP"
perl -i -pe 's/web:\s*\^1\.1\.0/web: 0.5.1/' "$APP"
perl -i -pe 's/wakelock_plus:\s*[^\n]+/wakelock_plus: 1.2.8/' "$APP"
perl -i -pe 's/video_player:\s*\^[^\n]+/video_player: 2.9.2/' "$APP"
perl -i -pe 's/media_kit_video:\s*[^\n]+/media_kit_video: 1.2.5/' "$APP"
perl -i -pe 's/flutter_html:\s*[^\n]+/flutter_html: 3.0.0-beta.2/' "$APP"

THEME="$ROOT/flutter/lib/theme/kotv_theme.dart"
if [[ -f "$THEME" ]]; then
  perl -i -pe 's/DialogThemeData\(/DialogTheme(/g' "$THEME"
  perl -i -pe 's/PredictiveBackPageTransitionsBuilder\(\)/ZoomPageTransitionsBuilder()/g' "$THEME"
fi

perl -i -0pe 's/\n  # 3\.44.*?\n  config:\n    enable-swift-package-manager: false\n/\n/s' "$APP" 2>/dev/null || true
perl -i -0pe 's/\n  config:\n    enable-swift-package-manager: false\n/\n/s' "$APP" 2>/dev/null || true

perl -i -0pe 's/\n# Win7[^\n]*\ndependency_overrides:[\s\S]*?(?=\ndev_dependencies:|\nflutter:|\Z)//' "$APP"
perl -i -0pe 's/\ndependency_overrides:[\s\S]*?(?=\ndev_dependencies:|\nflutter:|\Z)//' "$APP"

if ! grep -q 'family: NotoSansSC' "$APP"; then
  if grep -q 'family: NotoColorEmoji' "$APP"; then
    perl -i -0pe 's/(  fonts:\n)/$1    - family: NotoSansSC\n      fonts:\n        - asset: assets\/fonts\/NotoSansSC-Regular.otf\n          weight: 400\n        - asset: assets\/fonts\/NotoSansSC-Bold.otf\n          weight: 700\n    - family: NotoEmoji\n      fonts:\n        - asset: assets\/fonts\/NotoEmoji.ttf\n          weight: 400\n/s' "$APP"
  else
    perl -i -0pe 's/(  assets:\n    - assets\/engine\/\n)/$1  # Win7 only (adapt-flutter-win7-sdk.sh + fetch-flutter-fonts.sh)\n  fonts:\n    - family: NotoSansSC\n      fonts:\n        - asset: assets\/fonts\/NotoSansSC-Regular.otf\n          weight: 400\n        - asset: assets\/fonts\/NotoSansSC-Bold.otf\n          weight: 700\n    - family: NotoColorEmoji\n      fonts:\n        - asset: assets\/fonts\/NotoColorEmoji.ttf\n          weight: 400\n    - family: NotoEmoji\n      fonts:\n        - asset: assets\/fonts\/NotoEmoji.ttf\n          weight: 400\n/s' "$APP"
  fi
fi

perl -i -pe 's/path:\s*\^[^\n]+/path: 1.9.0/' "$APP"

cat >> "$APP" <<'OVERRIDES'

# Win7 / Flutter 3.19 临时覆盖（adapt-flutter-win7-sdk.sh；勿提交）
dependency_overrides:
  path: 1.9.0
  logging: 1.2.0
  ffi: 2.1.3
  path_provider: 2.1.4
  path_provider_android: 2.2.4
  path_provider_foundation: 2.4.0
  path_provider_linux: 2.2.1
  path_provider_windows: 2.3.0
  shared_preferences: 2.3.0
  shared_preferences_android: 2.2.2
  shared_preferences_foundation: 2.5.2
  shared_preferences_linux: 2.4.1
  shared_preferences_windows: 2.4.1
  video_player: 2.9.2
  video_player_android: 2.4.14
  video_player_platform_interface: 6.3.0
  video_player_avfoundation: 2.6.2
  video_player_web: 2.3.2
  web: 0.5.1
  wakelock_plus: 1.2.8
  media_kit_video: 1.2.5
  flutter_html: 3.0.0-beta.2
  html: 0.15.4
  url_launcher_windows: 3.1.3
  file_selector_windows: 0.9.3+3
OVERRIDES

echo "adapted pubspec for Flutter 3.19 / Dart 3.3"
grep -nE 'sdk:|video_player|media_kit_video|dependency_overrides' "$APP" | head -50
