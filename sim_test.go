package echoimage

import (
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

// The version changes with the code, content and modules a sim is built from and its tick rate, and with
// nothing else.
func TestSimVersionIsTheSourceWithoutTests(t *testing.T) {
	src := fstest.MapFS{
		"go.mod":                           {Data: []byte("module x\n")},
		"shards/game/main.go":              {Data: []byte("package main\n")},
		"shards/game/content/a/b.json":     {Data: []byte("{}")},
		"shards/game/main_test.go":         {Data: []byte("package main\n")},
		"shards/game/testdata/run.json":    {Data: []byte("[]")},
		"shards/game/internal/x/x.go":      {Data: []byte("package x\n")},
		"shards/game/internal/x/x_test.go": {Data: []byte("package x\n")},
	}
	with := func(path, data string) fstest.MapFS {
		c := maps.Clone(src)
		c[path] = &fstest.MapFile{Data: []byte(data)}
		return c
	}
	moved := maps.Clone(src)
	moved["shards/game/internal/y/x.go"] = moved["shards/game/internal/x/x.go"]
	delete(moved, "shards/game/internal/x/x.go")

	base := versionOf(src, 20)
	if !regexp.MustCompile(`^` + regexp.QuoteMeta(runtime.Version()) + `\+20hz\+[0-9a-f]{16}$`).MatchString(base) {
		t.Fatalf("version %q, want the Go release, +20hz+, and 16 hex digits", base)
	}
	for name, fsys := range map[string]fstest.MapFS{
		"the same files":   maps.Clone(src),
		"a test changed":   with("shards/game/internal/x/x_test.go", "package x // more\n"),
		"testdata changed": with("shards/game/testdata/run.json", "[1]"),
	} {
		if got := versionOf(fsys, 20); got != base {
			t.Errorf("%s: version %s, want %s", name, got, base)
		}
	}
	for name, fsys := range map[string]fstest.MapFS{
		"code changed":     with("shards/game/internal/x/x.go", "package x // more\n"),
		"content changed":  with("shards/game/content/a/b.json", `{"speed": 2}`),
		"a module changed": with("go.mod", "module x\n\nrequire y v1.0.1\n"),
		"a file added":     with("shards/game/internal/x/y.go", "package x\n"),
		"a file moved":     moved,
	} {
		if got := versionOf(fsys, 20); got == base {
			t.Errorf("%s: version unchanged", name)
		}
	}
	if versionOf(src, 60) == base {
		t.Error("another tick rate: version unchanged")
	}
}

// SimVersion hashes every file the shard and the predictor are built from, and the predictor doesn't
// embed the source itself: players never download the server's code.
func TestSimVersionCoversWhatTheSimIsBuiltFrom(t *testing.T) {
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"go.mod", "go.sum"} {
		if _, err := fs.Stat(source, file); err != nil {
			t.Errorf("SimVersion doesn't hash %s", file)
		}
	}
	for _, build := range []struct {
		pkg string
		env []string
	}{
		{"./shards/game", nil},
		{"./shards/game/predictor", []string{"GOOS=js", "GOARCH=wasm"}},
	} {
		// Each package of this module it builds: its import path, then its Go and embedded files.
		list := exec.Command("go", "list", "-deps", "-f", `{{if .Module}}{{if .Module.Main}}{{$dir := .Dir}}`+
			`{{.ImportPath}}{{range .GoFiles}} {{$dir}}/{{.}}{{end}}{{range .EmbedFiles}} {{$dir}}/{{.}}{{end}}{{end}}{{end}}`,
			build.pkg)
		list.Env = append(os.Environ(), build.env...)
		out, err := list.Output()
		if err != nil {
			t.Fatalf("go list %s: %v", build.pkg, err)
		}
		for line := range strings.Lines(string(out)) {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			if fields[0] == "echo_image" { // this package
				if strings.HasSuffix(build.pkg, "predictor") {
					t.Errorf("the predictor imports %s, so it would ship the source", fields[0])
				}
				continue
			}
			for _, file := range fields[1:] {
				rel, err := filepath.Rel(root, file)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := fs.Stat(source, filepath.ToSlash(rel)); err != nil {
					t.Errorf("%s builds from %s, which SimVersion doesn't hash: embed it in source", build.pkg, rel)
				}
			}
		}
	}
}
