// Package content holds the game shard's data-driven config (JSON), baked into the binary with
// go:embed, and Load, the one loader the shard and the client predictor share. Content is namespaced
// per feature: content/<feature>/<file>.json. Each feature defines its file's Go type, which names its
// file (JSONFile) and may bind (Bind) and check (Validate) itself.
package content

import (
	"embed"
	"encoding/json/v2"
	"fmt"
	"io/fs"
	"testing/fstest"
)

// FS holds every feature's JSON config. The glob matches one directory deep, so a new feature's
// content is picked up automatically once you add content/<feature>/<file>.json.
//
//go:embed */*.json
var FS embed.FS

// Overlay is the embedded content with these files, by path, in place of its own: a test's content.
type Overlay map[string][]byte

// Open opens name from o, or else from FS.
func (o Overlay) Open(name string) (fs.File, error) {
	if raw, ok := o[name]; ok {
		return fstest.MapFS{name: {Data: raw}}.Open(name)
	}
	return FS.Open(name)
}

// File is a content file's Go type.
type File interface{ JSONFile() string }

// Load decodes T's file from fsys strictly, so an unknown, repeated or wrong-case key fails, then binds
// and checks it. A returned error fails the boot.
func Load[T File](fsys fs.FS) (T, error) {
	var v T
	raw, err := fs.ReadFile(fsys, v.JSONFile())
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal(raw, &v, json.RejectUnknownMembers(true)); err != nil {
		return v, fmt.Errorf("%s: %w", v.JSONFile(), err)
	}
	if b, ok := any(&v).(interface{ Bind() error }); ok {
		if err := b.Bind(); err != nil {
			return v, fmt.Errorf("%s: %w", v.JSONFile(), err)
		}
	}
	if c, ok := any(&v).(interface{ Validate() error }); ok {
		if err := c.Validate(); err != nil {
			return v, fmt.Errorf("%s: %w", v.JSONFile(), err)
		}
	}
	return v, nil
}
