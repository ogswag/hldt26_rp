#!/usr/bin/env bash
# Builds the static demo: the app with no server behind it.
#
#   docker compose up -d --wait
#   scripts/ci/build-demo.sh [base-url]        # default http://localhost
#
# The demo carries three things a running installation would serve: the browser build of the calculation
# engine, the reference data (object types, their schemas, the frozen catalog with its photos) and the app
# itself, built with VITE_APP_MODE=demo so it reads those files instead of the API. The result is web/dist,
# ready for a bucket.
#
# It is a demo, not an installation: it saves nothing, has no accounts, and prints no run identifiers.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
base="${1:-http://localhost}"
public="$root/web/public"
demo="$public/demo"
engine="$public/engine"

cleanup() {
  rm -rf "$engine" "$demo"
}
trap cleanup EXIT

echo "==> engine"
rm -rf "$engine" "$demo"
mkdir -p "$engine" "$demo"
(cd "$root/api" && GOOS=js GOARCH=wasm go build -o "$engine/engine.wasm" ./cmd/engine-wasm)
cp "$(cd "$root/api" && go env GOROOT)/lib/wasm/wasm_exec.js" "$engine/wasm_exec.js"

get() {
  local path="$1" out="$2"
  curl --fail --silent --show-error "$base$path" -o "$demo/$out"
}

echo "==> reference data from $base"
get /api/object-types object-types.json
for type in $(node -e "process.stdout.write(require('$demo/object-types.json').items.map(t=>t.type).join(' '))"); do
  get "/api/object-types/$type/schema" "schema-$type.json"
done
get /api/catalog/dictionaries dictionaries.json
get "/api/solutions?limit=500" solutions.json
get /api/catalog/bundle catalog.json
for id in $(node -e "process.stdout.write(require('$demo/solutions.json').items.filter(s=>s.image_sha).map(s=>s.id).join(' '))"); do
  get "/api/solutions/$id/image" "image-$id"
done

echo "==> app"
(cd "$root/web" && VITE_APP_MODE=demo npm run build)

echo "==> done: $root/web/dist"
du -sh "$root/web/dist"
