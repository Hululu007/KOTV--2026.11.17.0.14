#!/usr/bin/env bash
# 统一 flutter pub get：Win7 / Dart 3.3 线自动先跑 adapt，避免 video_player_android 2.11 要 SDK ^3.10。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

_kotv_python() {
  if command -v python3 >/dev/null 2>&1; then
    python3 "$@"
  elif command -v python >/dev/null 2>&1; then
    python "$@"
  elif command -v py >/dev/null 2>&1; then
    py -3 "$@"
  else
    return 1
  fi
}

_kotv_pub_cache() {
  local p="${PUB_CACHE:-${PUB_CACHE_DIR:-$HOME/.pub-cache}}"
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -m "$p" 2>/dev/null || printf '%s\n' "$p"
  else
    printf '%s\n' "$p"
  fi
}

_kotv_pub_host_leaf() {
  case "${PUB_HOSTED_URL:-}" in
    *flutter-io.cn*) echo "pub.flutter-io.cn" ;;
    *) echo "pub.dev" ;;
  esac
}

_kotv_ensure_hosted_pkg() {
  local name="$1" ver="$2"
  local cache host_leaf dest url inner idir
  cache="$(_kotv_pub_cache)"
  host_leaf="$(_kotv_pub_host_leaf)"
  dest="$cache/hosted/$host_leaf/${name}-${ver}"
  if [[ -f "$dest/pubspec.yaml" ]]; then
    return 0
  fi
  echo "==> prefetch ${name}-${ver} into $dest"
  mkdir -p "$dest"
  url="https://pub.dev/api/archives/${name}-${ver}.tar.gz"
  if ! curl -fsSL -A 'KOTV-flutter-pub-get' "$url" | tar -xz -C "$dest"; then
    echo "ERROR: failed to prefetch ${name}-${ver} from $url" >&2
    rm -rf "$dest"
    return 1
  fi
  if [[ ! -f "$dest/pubspec.yaml" ]]; then
    inner="$(find "$dest" -maxdepth 2 -name pubspec.yaml | head -1 || true)"
    if [[ -n "$inner" ]]; then
      idir="$(dirname "$inner")"
      shopt -s dotglob
      mv "$idir"/* "$dest"/ 2>/dev/null || true
      shopt -u dotglob
    fi
  fi
  if [[ ! -f "$dest/pubspec.yaml" ]]; then
    echo "ERROR: prefetch ${name}-${ver} produced no pubspec.yaml" >&2
    rm -rf "$dest"
    return 1
  fi
  return 0
}

# Win7：放宽 video_player_platform_interface 6.3.x 的 environment。
_kotv_relax_win7_sdk_constraints() {
  local cache
  cache="$(_kotv_pub_cache)"
  _kotv_python - "$cache" <<'PY'
import pathlib, re, sys

cache = pathlib.Path(sys.argv[1]) / "hosted"
TARGET = '''environment:
  sdk: ">=3.3.0 <4.0.0"
  flutter: ">=3.19.0"'''
env_re = re.compile(r"environment:\n(?:  [^\n]+\n)+", re.MULTILINE)

def needs_relax(block: str) -> bool:
    sdk = re.search(r"sdk:\s*[\"']?([^\"'\n]+)", block)
    fl = re.search(r"flutter:\s*[\"']?([^\"'\n]+)", block)
    if fl and re.search(r">=\s*3\.(2[2-9]|[3-9][0-9])", fl.group(1)):
        return True
    if sdk and re.search(r"\^3\.(4|5|6|7|8|9|[1-9][0-9])", sdk.group(1)):
        return True
    if sdk and re.search(r">=\s*3\.(4|5|6|7|8|9|[1-9][0-9])", sdk.group(1)):
        return True
    return False

def relax_file(pubspec: pathlib.Path) -> bool:
    try:
        text = pubspec.read_text(encoding="utf-8")
    except OSError:
        return False
    m = env_re.search(text)
    if m and needs_relax(m.group(0)):
        text = text[: m.start()] + TARGET + "\n" + text[m.end() :]
        pubspec.write_text(text, encoding="utf-8")
        print(f"==> relaxed SDK constraints: {pubspec.parent.name}")
        return True
    return False

targets = []
if cache.is_dir():
    targets.extend(cache.glob("*/video_player_platform_interface-6.3.*/pubspec.yaml"))

if not targets:
    print("WARN: no video_player_platform_interface-6.3.* in pub-cache; skip relax" , file=sys.stderr)
    sys.exit(0)

patched = 0
for pub in sorted(set(targets)):
    if relax_file(pub):
        patched += 1
for pub in sorted(set(targets)):
    text = pub.read_text(encoding="utf-8")
    m = env_re.search(text)
    if m and needs_relax(m.group(0)):
        print(f"ERROR: still too new after relax: {pub.parent.name}", file=sys.stderr)
        sys.exit(1)
print(f"==> win7 SDK relax ok ({patched} written, {len(set(targets))} checked)")
PY
}

win7=0
if [[ "${KOTV_WIN7:-}" == "1" ]] || flutter --version 2>&1 | grep -qE 'Dart 3\.3\.|Flutter 3\.19'; then
  win7=1
  echo "==> Win7 / Dart 3.3: adapt pubspec before pub get"
  "$ROOT/scripts/adapt-flutter-win7-sdk.sh"
  _kotv_ensure_hosted_pkg video_player_platform_interface 6.3.0
  _kotv_relax_win7_sdk_constraints
  if [[ -f "$ROOT/flutter/pubspec.lock" ]]; then
    echo "==> Win7: remove pubspec.lock for clean resolve"
    rm -f "$ROOT/flutter/pubspec.lock"
  fi
fi

cd "$ROOT/flutter"
if [[ "${PUB_HOSTED_URL:-}" == "https://pub.dev" && -f pubspec.lock ]]; then
  if grep -q 'pub.flutter-io.cn' pubspec.lock 2>/dev/null; then
    echo "==> rewrite pubspec.lock hosted url → pub.dev"
    tmp="$(mktemp)"
    sed 's|https://pub.flutter-io.cn|https://pub.dev|g' pubspec.lock >"$tmp"
    mv "$tmp" pubspec.lock
  fi
fi
if ! flutter pub get "$@"; then
  if [[ "$win7" == "1" ]]; then
    echo "==> Win7 pub get failed; re-prefetch + relax and retry" >&2
    _kotv_ensure_hosted_pkg video_player_platform_interface 6.3.0
    _kotv_relax_win7_sdk_constraints
    flutter pub get "$@"
  elif [[ "${PUB_HOSTED_URL:-}" == *"flutter-io.cn"* ]]; then
    echo "WARN: pub get failed on $PUB_HOSTED_URL; retry pub.dev" >&2
    export PUB_HOSTED_URL=https://pub.dev
    export FLUTTER_STORAGE_BASE_URL="${FLUTTER_STORAGE_BASE_URL:-https://storage.googleapis.com}"
    if [[ -f pubspec.lock ]] && grep -q 'pub.flutter-io.cn' pubspec.lock 2>/dev/null; then
      tmp="$(mktemp)"
      sed 's|https://pub.flutter-io.cn|https://pub.dev|g' pubspec.lock >"$tmp"
      mv "$tmp" pubspec.lock
    fi
    flutter pub get "$@"
  else
    exit 1
  fi
fi
if [[ "$win7" == "1" ]]; then
  _kotv_relax_win7_sdk_constraints
fi
chmod +x "$ROOT/scripts/patch-media-kit.sh"
KOTV_WIN7="$win7" "$ROOT/scripts/patch-media-kit.sh"
