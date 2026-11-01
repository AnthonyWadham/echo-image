#!/usr/bin/env bash
# Every check this repo has, at both tick rates. `scripts/check.sh go` runs the Go half, which needs only
# Go; CI runs that (.github/workflows/check.yml). The client half needs bun and go-ecs beside this repo
# (the client's sdk alias) until the SDK's MessagePack codec is on npm. The pre-push hook runs it all:
# git config core.hooksPath .githooks
set -euo pipefail
cd "$(dirname "$0")/.."

unformatted=$(gofmt -l ./*.go shards pkg)
if [ -n "$unformatted" ]; then
	echo "gofmt: not formatted: $unformatted" >&2
	exit 1
fi
for tags in "" rate60; do
	go vet -tags "$tags" ./...
	GOOS=js GOARCH=wasm go vet -tags "$tags" ./shards/game/predictor
	go test -tags "$tags" ./...
done
if [ "${1:-}" = go ]; then
	exit 0
fi
cd client
bun run typecheck
GOFLAGS=-tags=rate60 bun run test
bun run test # last, so the predictor.wasm left behind is the default 20 Hz build
