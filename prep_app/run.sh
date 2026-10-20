#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
if [[ -x /opt/flutter/bin/flutter ]]; then
  export PATH="/opt/flutter/bin:${PATH}"
fi

cd "${ROOT}/backend"
go build -o prep-server ./cmd/server

if [[ ! -f "${ROOT}/app/build/web/index.html" ]]; then
  (cd "${ROOT}/app" && flutter build web --release)
fi

export KNOWLEDGE_JSON="${KNOWLEDGE_JSON:-${ROOT}/data/prep_app_knowledge_export.json}"

exec "${ROOT}/backend/prep-server" \
  -addr "${ADDR:-:8080}" \
  -web "${ROOT}/app/build/web"
