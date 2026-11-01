// Package echoimage says which build of the sim a binary runs, so the client's predictor can tell
// when it isn't the shard's.
package echoimage

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"runtime"
	"strings"
)

// source is what the sim is built from: the module's Go code and content, and the modules it
// requires. The shard's Docker build has no .git, so it can't stamp a commit instead.
//
//go:embed go.mod go.sum pkg shards
var source embed.FS

// SimVersion is the Go release, the tick rate and a hash of source without its tests: two builds with
// the same one run the same sim. rate is timestep.Rate, which a build tag picks and this package can't
// import. The shard sends it in body_sync; client/scripts/predictor.mjs stamps it into predictor.wasm.
func SimVersion(rate int) string { return versionOf(source, rate) }

// versionOf is SimVersion of the files in fsys at rate.
func versionOf(fsys fs.FS, rate int) string {
	h := sha256.New()
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && d.Name() == "testdata":
			return fs.SkipDir
		case d.IsDir() || strings.HasSuffix(path, "_test.go"):
			return nil
		}
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "%s %d\n", path, len(b))
		h.Write(b)
		return nil
	})
	if err != nil {
		panic(fmt.Sprintf("hashing the sim's source: %v", err))
	}
	return fmt.Sprintf("%s+%dhz+%s", runtime.Version(), rate, hex.EncodeToString(h.Sum(nil)[:8]))
}
