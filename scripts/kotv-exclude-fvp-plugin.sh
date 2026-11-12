#!/usr/bin/env bash
# 从 Flutter 插件列表去掉 fvp（桌面/iOS 仍可保留 pubspec 依赖）。
#
# 用法：
#   kotv-exclude-fvp-plugin.sh android   # 打包前清理；Gradle settings 还会再剥一次 deps
#   kotv-exclude-fvp-plugin.sh macos     # 仅当 KOTV_MACOS_NO_FVP=1
#
# 须在 flutter pub get 之后调用。安卓 APK 以 settings.gradle + kotv_android_no_fvp.gradle 为准。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
FLUTTER="$ROOT/flutter"
DEPS="$FLUTTER/.flutter-plugins-dependencies"
PLATFORM="${1:-}"

case "$PLATFORM" in
  android)
    echo "==> Android exclude fvp from Flutter plugin lists"
    ;;
  macos)
    if [[ "${KOTV_MACOS_NO_FVP:-0}" != "1" ]]; then
      echo "==> macOS keep fvp (KOTV_MACOS_NO_FVP=0)"
      exit 0
    fi
    echo "==> macOS exclude fvp (KOTV_MACOS_NO_FVP=1)"
    ;;
  *)
    echo "usage: $0 <android|macos>" >&2
    exit 1
    ;;
esac

if [[ ! -f "$DEPS" ]]; then
  echo "ERROR: missing $DEPS (run flutter pub get first)" >&2
  exit 1
fi

python3 - "$DEPS" "$PLATFORM" <<'PY'
import json, sys
from pathlib import Path

path = Path(sys.argv[1])
platform = sys.argv[2]
data = json.loads(path.read_text())
plugins = data.get("plugins", {})
before = plugins.get(platform, [])
after = [p for p in before if p.get("name") != "fvp"]
if len(after) == len(before):
    print(f"  - .flutter-plugins-dependencies[{platform}]: fvp already absent")
else:
    plugins[platform] = after
    data["plugins"] = plugins
    path.write_text(json.dumps(data, separators=(",", ":")))
    print(f"  - removed fvp from .flutter-plugins-dependencies[{platform}]")
PY

if [[ -f "$FLUTTER/.flutter-plugins" ]]; then
  if grep -q '^fvp=' "$FLUTTER/.flutter-plugins"; then
    grep -v '^fvp=' "$FLUTTER/.flutter-plugins" > "$FLUTTER/.flutter-plugins.tmp" || true
    mv "$FLUTTER/.flutter-plugins.tmp" "$FLUTTER/.flutter-plugins"
    echo "  - removed fvp from .flutter-plugins"
  fi
fi

case "$PLATFORM" in
  android)
    for cand in \
      "$FLUTTER/android/.android/plugins_build_output/fvp" \
      "$FLUTTER/.android/plugins_build_output/fvp"
    do
      if [[ -e "$cand" ]]; then
        rm -rf "$cand"
        echo "  - removed $cand"
      fi
    done
    # registrant 由 kotv_android_no_fvp.gradle 在 javac 前剥离；此处顺手清源树，避免半截工程。
    REG="$FLUTTER/android/app/src/main/java/io/flutter/plugins/GeneratedPluginRegistrant.java"
    if [[ -f "$REG" ]]; then
      python3 - "$REG" <<'PY'
from pathlib import Path
import re
path = Path(__import__("sys").argv[1])
text = path.read_text()
new = re.sub(
    r"\s*try \{\s*\n\s*flutterEngine\.getPlugins\(\)\.add\(new com\.mediadevkit\.fvp\.FvpPlugin\(\)\);\s*\n\s*\} catch \(Exception e\) \{\s*\n\s*Log\.e\(TAG, \"Error registering plugin fvp,.*?\n\s*\}\n",
    "\n",
    text,
    count=1,
    flags=re.S,
)
if new == text:
    if "FvpPlugin" not in text:
        print("  - GeneratedPluginRegistrant.java: FvpPlugin already absent")
    else:
        raise SystemExit("ERROR: FvpPlugin still in GeneratedPluginRegistrant but strip pattern missed")
else:
    path.write_text(new)
    print("  - patched GeneratedPluginRegistrant.java")
PY
    fi
    ;;
  macos)
    SYMLINK="$FLUTTER/macos/Flutter/ephemeral/.symlinks/plugins/fvp"
    if [[ -d "$SYMLINK" || -L "$SYMLINK" ]]; then
      rm -rf "$SYMLINK"
      echo "  - removed ephemeral fvp plugin symlink"
    fi
    REG="$FLUTTER/macos/Flutter/GeneratedPluginRegistrant.swift"
    if [[ -f "$REG" ]]; then
      python3 - "$REG" <<'PY'
from pathlib import Path
import re
path = Path(__import__("sys").argv[1])
text = path.read_text()
text2 = re.sub(r"^import fvp\n", "", text, flags=re.M)
text2 = re.sub(r"^\s*FvpPlugin\.register\(.*\)\n", "", text2, flags=re.M)
if text2 != text:
    path.write_text(text2)
    print("  - patched GeneratedPluginRegistrant.swift")
else:
    print("  - GeneratedPluginRegistrant.swift: fvp already absent")
PY
    fi
    ;;
esac
