#!/usr/bin/env bash
# flutter/assets/engine 里的引擎二进制会被 pubspec 打进 flutter_assets。
# 桌面和 Android 另有正式位置（Resources/engine、可执行文件旁、jniLibs）。
# 在 flutter build 前挪走，打完放回，本机 flutter run 仍能从源码目录找到引擎。
# 用法：source 本文件（调用方已设置 ROOT）。

kotv_flutter_asset_engines_hide() {
  local dir="${ROOT:?}/flutter/assets/engine"
  if [[ -n "${_KOTV_ENGINE_ASSET_STASH:-}" && -d "${_KOTV_ENGINE_ASSET_STASH}" ]]; then
    return 0
  fi
  _KOTV_ENGINE_ASSET_STASH="$(mktemp -d)"
  mkdir -p "$dir"
  KOTV_ENGINE_ASSET_DIR="$dir" KOTV_ENGINE_ASSET_STASH="$_KOTV_ENGINE_ASSET_STASH" python3 - <<'PY'
import os, shutil
src = os.environ["KOTV_ENGINE_ASSET_DIR"]
dst = os.environ["KOTV_ENGINE_ASSET_STASH"]
for name in ("kotv-engine", "kotv-engine.exe", "libkotv_engine.so"):
    path = os.path.join(src, name)
    if os.path.lexists(path):
        shutil.move(path, os.path.join(dst, name))
        print(f"==> leave {name} out of flutter assets", flush=True)
PY
}

kotv_flutter_asset_engines_restore() {
  local dir="${ROOT:?}/flutter/assets/engine"
  [[ -n "${_KOTV_ENGINE_ASSET_STASH:-}" && -d "${_KOTV_ENGINE_ASSET_STASH}" ]] || return 0
  mkdir -p "$dir"
  KOTV_ENGINE_ASSET_DIR="$dir" KOTV_ENGINE_ASSET_STASH="$_KOTV_ENGINE_ASSET_STASH" python3 - <<'PY'
import os, shutil
src = os.environ["KOTV_ENGINE_ASSET_STASH"]
dst = os.environ["KOTV_ENGINE_ASSET_DIR"]
for name in ("kotv-engine", "kotv-engine.exe", "libkotv_engine.so"):
    path = os.path.join(src, name)
    if not os.path.lexists(path):
        continue
    dest = os.path.join(dst, name)
    if os.path.lexists(dest):
        os.remove(dest)
    shutil.move(path, dest)
PY
  rm -rf "$_KOTV_ENGINE_ASSET_STASH"
  _KOTV_ENGINE_ASSET_STASH=""
}

# 目录：删掉 flutter_assets 里的引擎，再确认没有残留。
# 文件：当作 apk/ipa/zip，列出路径里不得再有这些文件。
kotv_flutter_asset_engines_reject() {
  local target name hits
  for target in "$@"; do
    if [[ -d "$target" ]]; then
      find "$target" -type f \( \
        -name kotv-engine -o -name kotv-engine.exe -o -name libkotv_engine.so \
      \) -path '*/assets/engine/*' -delete
      hits="$(find "$target" -type f \( \
        -name kotv-engine -o -name kotv-engine.exe -o -name libkotv_engine.so \
      \) -path '*/assets/engine/*' || true)"
      if [[ -n "$hits" ]]; then
        echo "ERROR: flutter_assets still has engine binaries under $target" >&2
        printf '%s\n' "$hits" >&2
        return 1
      fi
    elif [[ -f "$target" ]]; then
      hits="$(unzip -l "$target" | awk 'NR>3 { print $4 }' | grep -E '(^|/)assets/engine/(kotv-engine(\.exe)?|libkotv_engine\.so)$' || true)"
      if [[ -n "$hits" ]]; then
        echo "ERROR: $target still bundles engine binaries under flutter_assets" >&2
        printf '%s\n' "$hits" >&2
        return 1
      fi
    fi
  done
}
