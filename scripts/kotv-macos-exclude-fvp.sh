#!/usr/bin/env bash
# 兼容入口：转调 kotv-exclude-fvp-plugin.sh macos（KOTV_MACOS_NO_FVP=1 时排除）。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
exec "$ROOT/scripts/kotv-exclude-fvp-plugin.sh" macos
