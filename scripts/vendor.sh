#!/usr/bin/env bash
# Regenerate the vendor/ tree for the operator's Docker build.
#
# `world cardinal start` builds each shard in Docker from a PROJECT-ONLY context, so the local
# `replace github.com/argus-labs/world-engine => ../world-engine` can't resolve unless the module is
# vendored into the project. But `go mod vendor` only copies Go packages — it DROPS the physics2d
# Box2D C sources (a non-package `third_party/box2d/` tree pulled in via cgo `#include`), so the
# vendored cgo build fails with `box2d.h: No such file`. This script vendors, then restores Box2D.
#
# Run it after any dependency change and before `world cardinal start`. Never run a bare
# `go mod vendor` on its own — it leaves the Box2D sources missing.
set -euo pipefail
cd "$(dirname "$0")/.."

# Resolve the world-engine module dir (honors the ../world-engine replace) in module mode, BEFORE
# vendoring — once a vendor/ dir exists, `go list -m` no longer reports module dirs.
we_dir="$(go list -m -mod=mod -f '{{.Dir}}' github.com/argus-labs/world-engine)"
if [[ -z "$we_dir" || ! -d "$we_dir" ]]; then
	echo "error: could not resolve world-engine module dir (got: '$we_dir')" >&2
	exit 1
fi

go mod vendor

src="$we_dir/pkg/plugin/physics2d/third_party/box2d"
dst="vendor/github.com/argus-labs/world-engine/pkg/plugin/physics2d/third_party"
mkdir -p "$dst"
cp -r "$src" "$dst/"
test -f "$dst/box2d/include/box2d/box2d.h" || { echo "error: Box2D headers missing after copy" >&2; exit 1; }

echo "vendored + restored Box2D sources into $dst/box2d"
