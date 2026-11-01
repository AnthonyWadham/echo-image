#!/usr/bin/env bash
# Every check this repo has, at both tick rates: what CI would run. It needs Go, bun and go-ecs beside
# this repo (the client's sdk alias). The pre-push hook runs it: git config core.hooksPath .githooks
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
cd client
bun run typecheck
GOFLAGS=-tags=rate60 bun run test
bun run test # last, so the predictor.wasm left behind is the default 20 Hz build
