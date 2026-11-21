// Package queuetest checks a game's wiring of owner queues and system events: that each intent's
// senders run before the one system that drains its queue, and each system event's emitters before
// its readers. Cardinal can't check this from declarations yet, so [CheckOrder] reads the game's
// source for what each system sends, drains, emits and reads, and compares that with the order one
// tick runs its systems.
//
// The source must follow four rules, and CheckOrder fails where it can't tell:
//
//   - One system per file. A use belongs to the system declared in its file.
//   - Each send and emit is a literal (T{…}) or a conversion (T(x)), or [queue.Send] names T.
//   - Each [queue.Drain] names T, or its compare is a top-level func or a func literal.
//   - Each [queue.Declare] names T.
package queuetest

import (
	"bufio"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"echo_image/pkg/queue"
)

// The verbs of a use.
const (
	declares = "declares"
	drains   = "drains"
	emits    = "emits"
	reads    = "reads"
	sends    = "sends"
)

// CheckOrder ticks a world that register sets up once, reads the Go files under dirs (tests and the
// queue package aside), and fails t where a sender or emitter runs after its receiver, a queue has
// other than one drain, or a type is sent with no queue declared.
func CheckOrder(t testing.TB, register func(*cardinal.World), dirs ...string) {
	t.Helper()

	run := map[string]int{}
	for i, name := range systemsRun(t, register) {
		run[name] = i
	}

	by, declared := group(sourceUses(t, run, dirs))
	if len(declared) == 0 || len(by) == 0 {
		t.Fatalf("found %d queues and %d types used, want some: the scan missed the source", len(declared), len(by))
	}

	for typ := range declared {
		if d := by[typ][drains]; len(d) != 1 {
			t.Errorf("queue %s is drained by %v, want one system", typ, d)
		}
	}

	for typ, verbs := range by {
		if len(verbs[sends]) > 0 && !declared[typ] {
			t.Errorf("%v send %s, which no feature declares a queue for", verbs[sends], typ)
		}

		checkBefore(t, run, typ, verbs)
	}
}

// group returns the systems that use each type, by type and verb, and the types with a queue.
func group(uses []use) (map[string]map[string][]string, map[string]bool) {
	by, declared := map[string]map[string][]string{}, map[string]bool{}

	for _, u := range uses {
		if u.verb == declares {
			declared[u.typ] = true
			continue
		}

		if by[u.typ] == nil {
			by[u.typ] = map[string][]string{}
		}

		by[u.typ][u.verb] = append(by[u.typ][u.verb], u.system)
	}

	return by, declared
}

// checkBefore fails t where a system that emits or sends typ runs after one that reads or drains it.
func checkBefore(t testing.TB, run map[string]int, typ string, verbs map[string][]string) {
	t.Helper()

	for _, pair := range [][2]string{{emits, reads}, {sends, drains}} {
		for _, from := range verbs[pair[0]] {
			for _, to := range verbs[pair[1]] {
				if run[from] >= run[to] {
					t.Errorf("%s %s %s, but doesn't run before %s, which %s it", from, pair[0], typ, to, pair[1])
				}
			}
		}
	}
}

// systemsRun returns the package and type name of each system one tick runs past Init, in order.
func systemsRun(t testing.TB, register func(*cardinal.World)) []string {
	t.Helper()

	spans := tracetest.NewSpanRecorder()
	traced := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans))
	before := otel.GetTracerProvider()

	otel.SetTracerProvider(traced)
	t.Cleanup(func() {
		otel.SetTracerProvider(before)

		_ = traced.Shutdown(context.Background()) // before forwards to traced for good, so turn it off
	})
	cardinal.NewTestWorld(t, register).Tick()

	var systems []string

	for _, span := range spans.Ended() {
		var name, hook string

		for _, kv := range span.Attributes() {
			switch kv.Key {
			case "cardinal.system.name":
				name = strings.TrimPrefix(kv.Value.AsString(), "*")
			case "cardinal.system.hook":
				hook = kv.Value.AsString()
			}
		}

		if name != "" && hook != "SYSTEM_HOOK_INIT" {
			systems = append(systems, name)
		}
	}

	return systems
}

// use is what a system's file does with a type: emits or reads it as a system event, or sends or drains
// it as an intent. A queue's declaration is a use with no system.
type use struct{ system, verb, typ string }

// srcFile is a parsed Go file and its package's import path.
type srcFile struct {
	path, pkgPath string
	ast           *ast.File
	imports       map[string]string // import paths by local name
}

// funcIn is a top-level func's signature and the file it is declared in.
type funcIn struct {
	file *srcFile
	typ  *ast.FuncType
}

// scan is the game's source, as read for [sourceUses].
type scan struct {
	fset         *token.FileSet
	root, module string // the module's directory and path
	queuePath    string
	run          map[string]int
	files        []*srcFile
	typeNames    map[string]bool   // every type declared, by path and name
	funcs        map[string]funcIn // every top-level func, by path and name
	systemIn     map[string]string // the file each system of run is in
}

// sourceUses reads the Go files under dirs, but tests and the queue package, for each type a file
// emits, reads, sends, drains or declares a queue for. Every use but a declaration sits in the file of
// the one system of run that does it, and names its type.
func sourceUses(t testing.TB, run map[string]int, dirs []string) []use {
	t.Helper()
	root, module := moduleOf(t, dirs[0])

	s := &scan{
		fset:      token.NewFileSet(),
		root:      root,
		module:    module,
		queuePath: reflect.TypeFor[queue.Request]().PkgPath(),
		run:       run,
		typeNames: map[string]bool{},
		funcs:     map[string]funcIn{},
		systemIn:  map[string]string{},
	}
	for _, dir := range dirs {
		if err := filepath.WalkDir(dir, s.visit); err != nil {
			t.Fatal(err)
		}
	}

	systemOf := map[string]string{} // the system of run in each file
	for name, p := range s.systemIn {
		if other, ok := systemOf[p]; ok {
			t.Fatalf("%s holds two systems, %s and %s: give each its own file", p, other, name)
		}

		systemOf[p] = name
	}

	var uses []use
	for _, f := range s.files {
		uses = append(uses, s.usesIn(t, f, systemOf[f.path])...)
	}

	return uses
}

// visit parses the Go file at p, unless it is a test or in the queue package, and indexes its types
// and top-level funcs.
func (s *scan) visit(p string, d fs.DirEntry, err error) error {
	if err != nil || d.IsDir() || filepath.Ext(p) != ".go" || strings.HasSuffix(p, "_test.go") {
		return err
	}

	pkgPath, err := s.pkgPathOf(p)
	if err != nil {
		return err
	}

	if pkgPath == s.queuePath || strings.HasPrefix(pkgPath, s.queuePath+"/") {
		return nil
	}

	a, err := parser.ParseFile(s.fset, p, nil, parser.SkipObjectResolution)
	if err != nil {
		return err
	}

	f := &srcFile{path: p, pkgPath: pkgPath, ast: a, imports: importsOf(a)}

	s.files = append(s.files, f)
	for _, decl := range a.Decls {
		s.index(f, decl)
	}

	return nil
}

// pkgPathOf returns the import path of the package of the file at p.
func (s *scan) pkgPathOf(p string) (string, error) {
	abs, err := filepath.Abs(filepath.Dir(p))
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(s.root, abs)
	if err != nil {
		return "", err
	}

	return path.Join(s.module, filepath.ToSlash(rel)), nil
}

// index adds decl to the scan's top-level funcs, or its types and the files of systems of run.
func (s *scan) index(f *srcFile, decl ast.Decl) {
	switch decl := decl.(type) {
	case *ast.FuncDecl:
		if decl.Recv == nil {
			s.funcs[f.pkgPath+"."+decl.Name.Name] = funcIn{f, decl.Type}
		}
	case *ast.GenDecl:
		if decl.Tok != token.TYPE {
			return
		}

		for _, spec := range decl.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			s.typeNames[f.pkgPath+"."+ts.Name.Name] = true

			name := f.ast.Name.Name + "." + ts.Name.Name
			if _, isSystem := s.run[name]; isSystem {
				s.systemIn[name] = f.path
			}
		}
	}
}

// usesIn returns the uses in f, whose system is system or "", and fails t on a use it can't place.
func (s *scan) usesIn(t testing.TB, f *srcFile, system string) []use {
	t.Helper()

	var uses []use

	ast.Inspect(f.ast, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		verb, typ := s.useOf(f, call)
		at := s.fset.Position(call.Pos())

		switch {
		case verb == "":
		case typ == "":
			t.Errorf("%s: can't tell what this %s: name the type", at, verb)
		case verb == declares || system != "":
			uses = append(uses, use{system, verb, typ})
		default:
			t.Errorf("%s %s %s outside the file of a system a tick runs", at, verb, typ)
		}

		return true
	})

	return uses
}

// useOf returns what call does with which type, or "" for the verb if it is no use.
func (s *scan) useOf(f *srcFile, call *ast.CallExpr) (string, string) {
	fun, typ := call.Fun, ""
	if ix, ok := fun.(*ast.IndexExpr); ok {
		fun, typ = ix.X, typeIn(f, ix.Index)
	}

	sel, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return "", ""
	}

	x, _ := sel.X.(*ast.Ident)

	fromQueue := x != nil && f.imports[x.Name] == s.queuePath
	switch {
	case sel.Sel.Name == "EmitSystemEvent" && len(call.Args) == 1:
		return emits, s.valueType(f, call.Args[0])
	case sel.Sel.Name == "SystemEvents":
		return reads, typ
	case !fromQueue:
		return "", ""
	case sel.Sel.Name == "Send" || sel.Sel.Name == "SendAt":
		if typ == "" {
			typ = s.valueType(f, call.Args[len(call.Args)-1])
		}

		return sends, typ
	case sel.Sel.Name == "Drain":
		if typ == "" && len(call.Args) == 2 {
			typ = s.compareType(f, call.Args[1])
		}

		return drains, typ
	case sel.Sel.Name == "Declare":
		return declares, typ
	}

	return "", ""
}

// valueType returns the type of a literal T{…} or a conversion T(x) in f, or "".
func (s *scan) valueType(f *srcFile, e ast.Expr) string {
	switch e := e.(type) {
	case *ast.CompositeLit:
		return typeIn(f, e.Type)
	case *ast.CallExpr:
		if t := typeIn(f, e.Fun); len(e.Args) == 1 && s.typeNames[t] {
			return t
		}
	}

	return ""
}

// compareType returns R of a Drain's compare func(a, b R) int in f, as Go infers it, or "".
func (s *scan) compareType(f *srcFile, e ast.Expr) string {
	switch e := e.(type) {
	case *ast.FuncLit:
		return firstParam(f, e.Type)
	case *ast.Ident:
		if fn, ok := s.funcs[f.pkgPath+"."+e.Name]; ok {
			return firstParam(fn.file, fn.typ)
		}
	}

	return ""
}

// firstParam returns the type of ft's first parameter, declared in f, or "".
func firstParam(f *srcFile, ft *ast.FuncType) string {
	if ft.Params == nil || len(ft.Params.List) == 0 {
		return ""
	}

	return typeIn(f, ft.Params.List[0].Type)
}

// typeIn returns the import path and name of a type expression in f, or "".
func typeIn(f *srcFile, e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Ident:
		return f.pkgPath + "." + e.Name
	case *ast.SelectorExpr:
		if x, ok := e.X.(*ast.Ident); ok && f.imports[x.Name] != "" {
			return f.imports[x.Name] + "." + e.Sel.Name
		}
	}

	return ""
}

// importsOf returns a file's import paths by local name.
func importsOf(a *ast.File) map[string]string {
	imported := map[string]string{}

	for _, spec := range a.Imports {
		p, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}

		imported[path.Base(p)] = p
		if spec.Name != nil {
			imported[spec.Name.Name] = p
		}
	}

	return imported
}

// moduleOf returns the directory and path of the module that holds dir, from its go.mod.
func moduleOf(t testing.TB, dir string) (string, string) {
	t.Helper()

	root, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}

	for {
		module, err := modulePath(filepath.Join(root, "go.mod"))
		if err == nil {
			return root, module
		}

		if !os.IsNotExist(err) {
			t.Fatal(err)
		}

		parent := filepath.Dir(root)
		if parent == root {
			t.Fatalf("no go.mod holds %s", dir)
		}

		root = parent
	}
}

// modulePath reads the module line of the go.mod at p.
func modulePath(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()

	lines := bufio.NewScanner(f)
	for lines.Scan() {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(lines.Text()), "module"); ok {
			if rest = strings.TrimSpace(rest); strings.HasPrefix(rest, `"`) {
				return strconv.Unquote(rest)
			}

			return rest, nil
		}
	}

	if err := lines.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("%s has no module line", p)
}
