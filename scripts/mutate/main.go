// Command mutate breaks each rule in mutants.json once, in a temporary copy of the repo, and checks
// that a test catches it. It never touches the working tree. Run it from the repo root:
//
//	go run ./scripts/mutate          every mutant
//	go run ./scripts/mutate 12-G1 X8  only mutants whose id starts with one of these
//
// It fails when a mutant survives (unless it is marked survives), breaks the build, or no longer
// applies because its old text is gone.
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
)

type edit struct {
	File string `json:"file"`
	Old  string `json:"old"`
	New  string `json:"new"`
}

type mutant struct {
	ID       string `json:"id"`
	Breaks   string `json:"breaks"`
	Check    string `json:"check"`
	Edits    []edit `json:"edits"`
	Survives string `json:"survives,omitempty"` // why no test can catch it
}

type list struct {
	Checks  map[string]string `json:"checks"`
	Mutants []mutant          `json:"mutants"`
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "mutate:", err)
		os.Exit(1)
	}
}

func run(only []string) error {
	raw, err := os.ReadFile("scripts/mutate/mutants.json")
	if err != nil {
		return fmt.Errorf("run it from the repo root: %w", err)
	}
	var l list
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return err
	}
	var bad []string
	for _, m := range l.Mutants {
		if _, ok := l.Checks[m.Check]; !ok {
			bad = append(bad, m.ID+": unknown check "+m.Check)
		}
		for _, e := range m.Edits {
			src, err := os.ReadFile(e.File)
			if n := strings.Count(string(src), e.Old); err != nil || n != 1 {
				bad = append(bad, fmt.Sprintf("%s: old text found %d times in %s", m.ID, n, e.File))
			}
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("stale mutants, update mutants.json:\n  %s", strings.Join(bad, "\n  "))
	}

	tmp, err := os.MkdirTemp("", "mutate-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	go func() { <-stop; os.RemoveAll(tmp); os.Exit(130) }()
	root := filepath.Join(tmp, filepath.Base(must(os.Getwd())))
	files, err := copyTree(".", root)
	if err != nil {
		return err
	}

	var failed []string
	for _, m := range l.Mutants {
		if !wanted(m.ID, only) {
			continue
		}
		verdict, out := try(root, m, l.Checks[m.Check])
		restore(root, files)
		fmt.Printf("%-10s %-9s %s\n", m.ID, verdict, m.Breaks)
		if verdict == "survived" && m.Survives != "" {
			continue
		}
		if verdict != "caught" {
			failed = append(failed, m.ID+" "+verdict)
			if verdict == "broken" {
				fmt.Println(indent(out))
			}
		}
		if verdict == "caught" && m.Survives != "" {
			failed = append(failed, m.ID+" is caught now: drop its survives note")
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d mutants need a look:\n  %s", len(failed), strings.Join(failed, "\n  "))
	}
	return nil
}

// try applies m in the copy at root and runs its check there.
func try(root string, m mutant, check string) (verdict, out string) {
	for _, e := range m.Edits {
		p := filepath.Join(root, e.File)
		src := must(os.ReadFile(p))
		must0(os.WriteFile(p, []byte(strings.Replace(string(src), e.Old, e.New, 1)), 0o644))
	}
	cmd := exec.Command("sh", "-c", check)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOFLAGS=-count=1")
	b, err := cmd.CombinedOutput()
	switch out = string(b); {
	case strings.Contains(out, "[build failed]") || strings.Contains(out, "[setup failed]"):
		return "broken", out
	case err != nil:
		return "caught", out
	}
	return "survived", out
}

// restore puts back every file of the copy that a mutant or its check changed (an -update rewrites
// testdata).
func restore(root string, files map[string][]byte) {
	for p, src := range files {
		to := filepath.Join(root, p)
		if cur, err := os.ReadFile(to); err != nil || string(cur) != string(src) {
			must0(os.WriteFile(to, src, 0o644))
		}
	}
}

// copyTree copies the repo to dst without .git and dist, and returns what it copied. node_modules and
// go-ecs (the client's SDK alias) are linked, not copied.
func copyTree(src, dst string) (map[string][]byte, error) {
	abs := must(filepath.Abs(src))
	if ecs := filepath.Join(filepath.Dir(abs), "go-ecs"); exists(ecs) {
		must0(os.Symlink(ecs, filepath.Join(filepath.Dir(dst), "go-ecs")))
	}
	files := map[string][]byte{}
	return files, filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		to := filepath.Join(dst, p)
		switch {
		case d.Name() == ".git" || d.Name() == "dist":
			return filepath.SkipDir
		case d.Name() == "node_modules":
			return errSkip(os.Symlink(filepath.Join(abs, p), to))
		case d.IsDir():
			return os.MkdirAll(to, 0o755)
		case d.Type().IsRegular():
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			files[p] = b
			return os.WriteFile(to, b, must(d.Info()).Mode().Perm())
		}
		return nil
	})
}

func errSkip(err error) error {
	if err != nil {
		return err
	}
	return filepath.SkipDir
}

func wanted(id string, only []string) bool {
	if len(only) == 0 {
		return true
	}
	for _, o := range only {
		if strings.HasPrefix(id, o) {
			return true
		}
	}
	return false
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func indent(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 12 {
		lines = lines[:12]
	}
	return "    " + strings.Join(lines, "\n    ")
}

func must[T any](v T, err error) T {
	must0(err)
	return v
}

func must0(err error) {
	if err != nil {
		panic(err)
	}
}
