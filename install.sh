#!/usr/bin/env bash
# Install the SDK, its agent skill and an optional personal Selectel profile.
set +x +v
set -euo pipefail
umask 077

for dependency in git python3; do
  if ! command -v "$dependency" >/dev/null 2>&1; then
    printf 'Для установки нужен %s. Установите его и повторите команду.\n' "$dependency" >&2
    exit 1
  fi
done

installer_source=""
installer_temp=""
trap 'if [[ -n "$installer_temp" ]]; then rm -rf -- "$installer_temp"; fi' EXIT

installer_script=${BASH_SOURCE[0]:-}
if [[ -f "$installer_script" ]]; then
  installer_source=$(cd -- "$(dirname -- "$installer_script")" && pwd)
fi
if [[ -z "$installer_source" || ! -f "$installer_source/scripts/local_setup.py" ]]; then
  installer_temp=$(mktemp -d)
  installer_source="$installer_temp/sdk"
  printf 'Загрузка selectelsdk…\n'
  git init --quiet "$installer_source"
  git -C "$installer_source" fetch --quiet --depth 1 \
    -- https://github.com/festy23/selectel-sdk.git "${SELECTELSDK_REF:-main}"
  git -C "$installer_source" checkout --quiet --detach FETCH_HEAD
fi

python3 "$installer_source/scripts/local_setup.py" install \
  --source "$installer_source" "$@"
