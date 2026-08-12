#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
unit_dir="${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user"
mkdir -p "$unit_dir"
install -m 0644 "$project_dir/deploy/systemd/ai-workbench-api.service" "$unit_dir/ai-workbench-api.service"
install -m 0644 "$project_dir/deploy/systemd/ai-workbench-ui.service" "$unit_dir/ai-workbench-ui.service"
systemctl --user daemon-reload
systemctl --user enable ai-workbench-api.service ai-workbench-ui.service
systemctl --user restart ai-workbench-api.service ai-workbench-ui.service
systemctl --user --no-pager --full status ai-workbench-api.service ai-workbench-ui.service
