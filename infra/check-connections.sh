#!/usr/bin/env bash
# Verify Phase 1 infra connections on nuc.test.
# Usage: infra/check-connections.sh   (reads infra/.env if present, else infra/.env.example)
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$DIR/.env"; [ -f "$ENV_FILE" ] || ENV_FILE="$DIR/.env.example"
set -a; . "$ENV_FILE"; set +a

fail=0

echo "== Postgres $POSTGRES_HOST:$POSTGRES_PORT =="
if PGPASSWORD="$POSTGRES_PASSWORD" psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" \
     -U "$POSTGRES_USER" -d "$POSTGRES_DB" -tAc "select 1" >/dev/null 2>&1; then
  echo "  OK"
else
  echo "  FAIL"; fail=1
fi

echo "== Redis $REDIS_HOST:$REDIS_PORT =="
if python3 - "$REDIS_HOST" "$REDIS_PORT" "$REDIS_PASSWORD" <<'PY'
import socket,sys
h,p,pw=sys.argv[1],int(sys.argv[2]),sys.argv[3]
s=socket.create_connection((h,p),3); s.settimeout(2)
s.sendall(f"AUTH {pw}\r\nPING\r\n".encode())
sys.exit(0 if b"PONG" in s.recv(100) else 1)
PY
then echo "  OK"; else echo "  FAIL"; fail=1; fi

echo "== MinIO $MINIO_ENDPOINT =="
scheme=http; [ "${MINIO_USE_SSL:-false}" = "true" ] && scheme=https
if [ "$(curl -s -o /dev/null -w '%{http_code}' "$scheme://$MINIO_ENDPOINT/minio/health/live")" = "200" ]; then
  echo "  OK (health 200)"
else
  echo "  FAIL"; fail=1
fi

exit $fail
