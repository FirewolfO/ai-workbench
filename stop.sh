#!/usr/bin/env bash
set -euo pipefail
project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
for name in backend frontend; do
  pid_file="$project_dir/.runtime/$name.pid"
  if [[ -f "$pid_file" ]]; then
    pid="$(<"$pid_file")"
    if kill -0 "$pid" 2>/dev/null; then kill "$pid"; fi
  fi
done
echo "AI Workbench services stopped"
