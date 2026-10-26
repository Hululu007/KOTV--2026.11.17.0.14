#!/usr/bin/env bash
# 单 ABI APK 里不得再有多余目录、空目录、空文件。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec "$ROOT/scripts/tidy-android-apk.sh" --check "$@"
