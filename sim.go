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
	"runtime/debug"
	"strings"
)

// source is what the sim is built from: the modules it requires, and the shard's and predictor's Go code
// and content, not the shard's tests and testdata. The shard's Docker build has no .git, so it can't
// stamp a commit instead.
//
//go:embed go.mod go.sum
//go:embed shards/game/main.go shards/game/shard/shard.go shards/game/shard/catalog.go
//go:embed shards/game/content shards/game/internal
//go:embed shards/game/predictor
var source embed.FS

// SimVersion is the Go release, the build tags (rate60 picks 60 Hz) and a hash of source without its
// tests: two builds with the same one run the same sim. The shard sends it in body_sync;
// client/scripts/predictor.mjs stamps it into predictor.wasm.
func SimVersion() string { return versionOf(source, buildTags()) }

// buildTags is the -tags this binary was built with.
func buildTags() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "-tags" {
				return s.Value
			}
		}
	}
	return ""
}

// versionOf is SimVersion of the files in fsys, built with tags.
func versionOf(fsys fs.FS, tags string) string {
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
	v := runtime.Version()
	if tags != "" {
		v += "+" + tags
	}
	return v + "+" + hex.EncodeToString(h.Sum(nil)[:8])
}
