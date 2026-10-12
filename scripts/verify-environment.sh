#!/bin/sh
set -eu

repository_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repository_root"

project_name=pulse-backend-t01-test
compose_files="-f docker-compose.yml -f docker-compose.test.yml"
legacy_log=$(mktemp "${TMPDIR:-/tmp}/pulse-legacy-guard.XXXXXX")
readiness_body=$(mktemp "${TMPDIR:-/tmp}/pulse-readiness.XXXXXX")

compose() {
    # shellcheck disable=SC2086
    docker compose -p "$project_name" $compose_files "$@"
}

cleanup() {
    compose down --volumes --remove-orphans >/dev/null 2>&1 || true
    rm -f "$legacy_log" "$readiness_body"
}
trap cleanup EXIT INT TERM

# Only this explicitly named, isolated test project is removed. The regular
# pulse-backend volumes and any user database are never addressed here.
cleanup
trap cleanup EXIT INT TERM

compose config --quiet
compose up --build --detach

attempt=0
until curl --fail --silent --show-error http://127.0.0.1:18080/health/ready >/dev/null; do
    attempt=$((attempt + 1))
    if [ "$attempt" -ge 45 ]; then
        compose ps
        compose logs --no-color backend db db-schema minio minio-init
        echo "backend readiness check timed out" >&2
        exit 1
    fi
    sleep 2
done

curl --fail --silent --show-error http://127.0.0.1:18080/health/live >/dev/null
compose exec -T db psql -X -v ON_ERROR_STOP=1 -U pulse_test -d pulse_test \
    < scripts/verify-schema.sql

S3_ENDPOINT=http://127.0.0.1:19000 \
S3_PUBLIC_ENDPOINT=http://127.0.0.1:19000 \
S3_BUCKET=pulse-test \
S3_ACCESS_KEY=pulse-test \
S3_SECRET_KEY=pulse-test-secret \
    go test -count=1 -tags=integration ./internal/storage/s3

# The API must become unready when photo storage is unavailable and must not
# expose the provider error or its private endpoint in the response.
compose stop minio >/dev/null
http_code=$(curl --max-time 10 --silent --output "$readiness_body" --write-out '%{http_code}' \
    http://127.0.0.1:18080/health/ready)
test "$http_code" = "503"
grep -q 'dependency is unavailable' "$readiness_body"
if grep -Eq 'minio|9000|S3|secret' "$readiness_body"; then
    echo "readiness response leaked storage details" >&2
    exit 1
fi
compose start minio >/dev/null
attempt=0
until curl --fail --silent --show-error http://127.0.0.1:18080/health/ready >/dev/null; do
    attempt=$((attempt + 1))
    [ "$attempt" -lt 30 ] || { echo "MinIO recovery timed out" >&2; exit 1; }
    sleep 2
done

compose exec -T minio /bin/sh -ec '
    mc alias set pulse-local http://127.0.0.1:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null
    printf "%s" "object storage persistence marker" | mc pipe pulse-local/"$S3_BUCKET"/persistence-marker.txt >/dev/null
    mc stat pulse-local/"$S3_BUCKET"/persistence-marker.txt >/dev/null
'

# Restart the stateful services, then the API, and prove both stores retained
# the markers. This catches an incorrect PostgreSQL 18 mount immediately.
compose restart db minio >/dev/null
attempt=0
until compose exec -T db pg_isready -U pulse_test -d pulse_test >/dev/null 2>&1; do
    attempt=$((attempt + 1))
    [ "$attempt" -lt 30 ] || { echo "postgres restart timed out" >&2; exit 1; }
    sleep 2
done
compose restart backend >/dev/null

attempt=0
until curl --fail --silent --show-error http://127.0.0.1:18080/health/ready >/dev/null; do
    attempt=$((attempt + 1))
    [ "$attempt" -lt 30 ] || { echo "backend restart timed out" >&2; exit 1; }
    sleep 2
done

test "$(compose exec -T db psql -X -Atqc \
    "SELECT count(*) FROM users WHERE email = 'one@example.test'" \
    -U pulse_test -d pulse_test | tr -d '\r')" = "1"
compose exec -T minio /bin/sh -ec '
    mc alias set pulse-local http://127.0.0.1:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null
    mc stat pulse-local/"$S3_BUCKET"/persistence-marker.txt >/dev/null
'

# Verify that decisions_002 refuses a non-empty legacy database and leaves its
# row, BIGINT identifier and migration history intact.
compose exec -T db psql -X -v ON_ERROR_STOP=1 -U pulse_test -d postgres \
    -c 'CREATE DATABASE pulse_legacy_guard' >/dev/null
compose exec -T db psql -X -v ON_ERROR_STOP=1 -U pulse_test -d pulse_legacy_guard \
    -f /docker-entrypoint-initdb.d/001_initial.sql >/dev/null
compose exec -T db psql -X -v ON_ERROR_STOP=1 -U pulse_test -d pulse_legacy_guard \
    -c "INSERT INTO users(email, password_hash) VALUES ('legacy@example.test', 'hash')" >/dev/null

if compose exec -T db psql -X -v ON_ERROR_STOP=1 -U pulse_test -d pulse_legacy_guard \
    -f /docker-entrypoint-initdb.d/002_decisions.sql >"$legacy_log" 2>&1; then
    echo "decisions_002 unexpectedly accepted legacy data" >&2
    exit 1
fi
grep -q 'Legacy data exists' "$legacy_log"

test "$(compose exec -T db psql -X -Atqc \
    "SELECT count(*) FROM users WHERE email = 'legacy@example.test'" \
    -U pulse_test -d pulse_legacy_guard | tr -d '\r')" = "1"
test "$(compose exec -T db psql -X -Atqc \
    "SELECT data_type FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'uid'" \
    -U pulse_test -d pulse_legacy_guard | tr -d '\r')" = "bigint"
test "$(compose exec -T db psql -X -Atqc \
    "SELECT string_agg(version, ',' ORDER BY version) FROM schema_migrations" \
    -U pulse_test -d pulse_legacy_guard | tr -d '\r')" = "initial_001"

compose exec -T db psql -X -v ON_ERROR_STOP=1 -U pulse_test -d postgres \
    -c 'DROP DATABASE pulse_legacy_guard' >/dev/null

echo "T01/T07 environment and photo storage verification passed"
