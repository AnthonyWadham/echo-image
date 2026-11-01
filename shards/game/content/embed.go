// Package content holds the game shard's data-driven config (JSON), baked into the binary with
// go:embed, and Load, the one loader the shard and the client predictor share. Content is namespaced
// per feature: content/<feature>/<file>.json. Each feature defines its file's Go type, which names its
// file (JSONFile) and may bind (Bind) and check (Validate) itself.
package content

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
)

// FS holds every feature's JSON config. The glob matches one directory deep, so a new feature's
// content is picked up automatically once you add content/<feature>/<file>.json.
//
//go:embed */*.json
var FS embed.FS

// File is a content file's Go type.
type File interface{ JSONFile() string }

// Load decodes T's file from fsys strictly, so an unknown key fails, then binds and checks it. A
// returned error fails the boot.
func Load[T File](fsys fs.FS) (T, error) {
	var v T
	raw, err := fs.ReadFile(fsys, v.JSONFile())
	if err != nil {
		return v, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return v, fmt.Errorf("%s: %w", v.JSONFile(), err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return v, fmt.Errorf("%s: data after the object", v.JSONFile())
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
