#!/usr/bin/env bash
set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
runtime_dir="$project_dir/.runtime"
mkdir -p "$runtime_dir/bin"

if command -v lsof >/dev/null 2>&1; then
  for port in 8087 5181; do
    if lsof -nP -t -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
      echo "Port $port is already in use; stop that service or override the development port." >&2
      exit 1
    fi
  done
fi

(cd "$project_dir/backend" && go build -o "$runtime_dir/bin/ai-workbench-server" ./cmd/server)

backend_pid=""
frontend_pid=""
cleanup() {
  [[ -n "$backend_pid" ]] && kill "$backend_pid" 2>/dev/null || true
  [[ -n "$frontend_pid" ]] && kill "$frontend_pid" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

(
  cd "$project_dir/backend"
  if [[ -f .env ]]; then set -a; source .env; set +a; fi
  exec "$runtime_dir/bin/ai-workbench-server"
) >"$runtime_dir/backend.log" 2>&1 &
backend_pid=$!
echo "$backend_pid" >"$runtime_dir/backend.pid"

(cd "$project_dir/frontend" && exec npm run dev -- --host 0.0.0.0 --port 5181 --strictPort) >"$runtime_dir/frontend.log" 2>&1 &
frontend_pid=$!
echo "$frontend_pid" >"$runtime_dir/frontend.pid"

ready=false
for _ in {1..40}; do
  if curl --fail --silent http://127.0.0.1:8087/health >/dev/null && curl --fail --silent http://127.0.0.1:5181/ >/dev/null; then
    echo "AI Workbench API is ready at http://127.0.0.1:8087"
    echo "AI Workbench UI is ready at http://127.0.0.1:5181"
    ready=true
    break
  fi
  sleep 0.25
done
if [[ "$ready" != true ]]; then
  echo "AI Workbench did not become ready. Check $runtime_dir/*.log" >&2
  exit 1
fi
wait -n "$backend_pid" "$frontend_pid"
