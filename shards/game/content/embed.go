// Package content bakes the game shard's data-driven config (JSON) into the binary via go:embed.
// main.go hands FS to the data plugin; feature packages define the typed schema (a data.Definition)
// and read values back with data.Get[T](). Content is namespaced per feature: content/<feature>/*.json.
package content

import "embed"

// FS holds every feature's JSON config. The glob matches one directory deep, so a new feature's
// content is picked up automatically once you add content/<feature>/<file>.json.
//
//go:embed */*.json
var FS embed.FS
