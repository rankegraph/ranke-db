#!/usr/bin/env sh
# smoke.sh — launch ranke-db against the minimal example, seed it over its own REST API,
# read the graph back, and shut down. A self-test that the whole pipeline works end to
# end: load → resolve → assemble → serve, then contribute → merge → query.
# Arg 1: the ranke-db binary. Arg 2: the generator binary.
set -eu

BIN="${1:?usage: smoke.sh <ranke-db-binary> <generator-binary>}"
GEN="${2:?usage: smoke.sh <ranke-db-binary> <generator-binary>}"
CONFIG="examples/minimal/config.json"
PORT="8089"
ADDR="127.0.0.1:$PORT"
URL="http://$ADDR"

if ! command -v openssl >/dev/null 2>&1; then
	echo "smoke: openssl required to generate a throwaway signer key" >&2
	exit 1
fi

# A throwaway Ed25519 identity for the smoke run, supplied via env() — the
# adapter never generates keys; this is the operator providing one.
RANKE_SIGNER_KEY="$(openssl genpkey -algorithm ed25519)"
export RANKE_SIGNER_KEY

# And a first contributor to found the archive under. Only its public half reaches
# the config: an archive comes into being once, under a key the server never holds.
RANKE_FOUNDER_PUBKEY="$(printf '%s' "$(openssl genpkey -algorithm ed25519)" | openssl pkey -pubout)"
export RANKE_FOUNDER_PUBKEY

# The configuration is the composition root: an endpoint listens where its own
# section says, so the smoke port is set by editing the config, not by a flag. The
# admin socket moves under $WORK too, so smoke touches nothing outside its own tmpdir.
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
mkdir -m 0700 "$WORK/run"
sed -e "s/\":8080\"/\"$ADDR\"/" -e "s#\"unix://./run/admin.sock\"#\"unix://$WORK/run/admin.sock\"#" "$CONFIG" > "$WORK/config.json"

echo ">> launching: $BIN run <minimal example on $ADDR>"
"$BIN" run "$WORK/config.json" &
PID=$!
trap 'kill "$PID" 2>/dev/null || true; rm -rf "$WORK"' EXIT

# Wait for the listener before asserting anything about what it answers.
code=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
	code="$(curl -s -o /dev/null -m 2 -w '%{http_code}' "$URL/health" || true)"
	case "$code" in
	000 | "") sleep 0.3 ;;
	*) break ;;
	esac
done

case "$code" in
000 | "") echo "smoke: no answer from GET /health on $ADDR" >&2; exit 1 ;;
200) ;;
*) echo "smoke: GET /health returned $code, want 200" >&2; exit 1 ;;
esac
echo ">> GET /health OK"

# Seeding runs as a client, the way an application contributes: claims signed with its
# own key, sent as a contribution stream. This is the write path's end-to-end check.
echo ">> seeding over POST /contribute"
"$GEN" example "$URL" --wait 5s

# A client that knows no branch name starts here. Typed unquoted, as these routes are
# pinned to be.
echo ">> GET /branches discovers what was seeded"
branches="$(curl -s -m 5 "$URL/branches")"
if ! printf '%s' "$branches" | grep -q '"name":"main"'; then
	echo "smoke: GET /branches did not list the seeded branch: $branches" >&2
	exit 1
fi
echo ">> branch listing OK"

# And what was written reads back. The seeded entity is the one claim whose content is
# distinctive enough to find in the result without parsing the sequence.
echo ">> POST /query reads the seeded graph back"
body="$(curl -s -m 10 -X POST "$URL/query" \
	-H 'content-type: application/json' \
	-d '{"select":{"branch":"main"},"output":{"encoding":"json"}}')"
if ! printf '%s' "$body" | grep -q '"type":"entity/person"'; then
	echo "smoke: the seeded entity/person claim did not come back from /query" >&2
	printf 'smoke: got %s\n' "$body" >&2
	exit 1
fi
echo ">> seeded claims read back OK"

# The release shape writes what `example` cannot: claims several attested identities signed,
# and logs long enough to fetch. One release keeps this quick — the point is the paths, not
# the size. What a slide draws is only presentable if a server accepts it.
echo ">> seeding the release scenario"
"$GEN" release --releases 1 "$URL" >/dev/null

echo ">> POST /query finds a claim an attested identity signed"
release="$(curl -s -m 10 -X POST "$URL/query" \
	-H 'content-type: application/json' \
	-d '{"select":{"branch":"main"},"output":{"detail":"claims","encoding":"json"},
	     "where":{"field":"type","test":{"eq":"derivation/release"}}}')"
# Two triage decisions meet here: the fan-in is what the scenario exists to show.
if [ "$(printf '%s' "$release" | grep -o 'derivation/input' | wc -l)" -lt 2 ]; then
	echo "smoke: the release claim cites fewer than two decisions: $release" >&2
	exit 1
fi
echo ">> the release fans in from both packages OK"

echo ">> GET a build log through the content route"
log_id="$(curl -s -m 10 -X POST "$URL/query" \
	-H 'content-type: application/json' \
	-d '{"select":{"branch":"main"},"output":{"detail":"claims","encoding":"json"},
	     "where":{"field":"type","test":{"eq":"source/build_log"}},"limit":{"results":1}}' \
	| grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)"
if [ -z "$log_id" ]; then
	echo "smoke: no build log came back from /query" >&2
	exit 1
fi
bytes="$(curl -s -m 10 "$URL/branches/main/claims/$log_id/content" | wc -c)"
if [ "$bytes" -lt 1024 ]; then
	echo "smoke: the content route served $bytes bytes, want a log worth capping" >&2
	exit 1
fi
echo ">> content route served $bytes bytes OK"

# The boundary check runs ahead of core: a query naming no scope is a 400 whatever the
# engine behind it does.
code="$(curl -s -o /dev/null -m 5 -w '%{http_code}' -X POST "$URL/query" \
	-H 'content-type: application/json' -d '{"select":{}}')"
if [ "$code" != "400" ]; then
	echo "smoke: POST /query with no scope returned $code, want 400" >&2
	exit 1
fi
echo ">> POST /query rejects a scopeless query (400) OK"

echo ">> shutting down"
kill "$PID" 2>/dev/null || true
wait "$PID" 2>/dev/null || true
trap 'rm -rf "$WORK"' EXIT
echo ">> smoke OK"
