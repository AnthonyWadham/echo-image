# Queue research: owner-write analyzer prototype

Built 2026-11-19 during the [queue API research](../queue-api.md), where it is source [5]. It reports
`Entity.Set` or `Entity.Remove` on a component type declared in another package. It ran on its test data
only. The type-instance probe at the end checked that `go/types` records the type arguments of generic
method calls, which the analyzer relies on.

## ownercheck.go

```go
// Package ownercheck reports Entity.Set/Remove calls on a component type declared in another package.
package ownercheck

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

var Analyzer = &analysis.Analyzer{
	Name: "ownercheck",
	Doc:  "reports writes to a component owned by another package",
	Run:  run,
}

// writeMethods are the generic Entity methods that write a component.
var writeMethods = map[string]bool{"Set": true, "Remove": true}

func run(pass *analysis.Pass) (any, error) {
	for id, inst := range pass.TypesInfo.Instances {
		fn, ok := pass.TypesInfo.Uses[id].(*types.Func)
		if !ok || !writeMethods[fn.Name()] {
			continue
		}
		recv := fn.Signature().Recv()
		if recv == nil {
			continue
		}
		rn, ok := types.Unalias(recv.Type()).(*types.Named)
		if !ok || rn.Obj().Name() != "Entity" || rn.Obj().Pkg().Name() != "ecs" {
			continue
		}
		comp, ok := types.Unalias(inst.TypeArgs.At(0)).(*types.Named)
		if !ok || comp.Obj().Pkg() == pass.Pkg {
			continue
		}
		pass.Reportf(id.Pos(), "%s.%s writes %s, owned by package %s; send a request to its owner",
			rn.Obj().Name(), fn.Name(), comp.Obj().Name(), comp.Obj().Pkg().Path())
	}
	return nil, nil
}

var _ = ast.Ident{}
```

## ownercheck_test.go

```go
package ownercheck_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/analysis/passes/unusedwrite"

	"ownercheck"
)

func TestOwner(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), ownercheck.Analyzer, "ai", "combat")
}

func TestUnusedWrite(t *testing.T) {
	r := analysistest.Run(t, analysistest.TestData(), unusedwrite.Analyzer, "ai")
	for _, res := range r {
		for _, d := range res.Diagnostics {
			t.Logf("unusedwrite: %s %s", res.Pass.Fset.Position(d.Pos), d.Message)
		}
	}
}
```

## testdata/src/ecs/ecs.go

```go
package ecs

type Entity struct{}

func (e Entity) Set[T any](c T)    {}
func (e Entity) Get[T any]() (z T) { return }
func (e Entity) Remove[T any]()    {}
```

## testdata/src/combat/combat.go

```go
package combat

import "ecs"

type Health struct{ HP int }

func Heal(e ecs.Entity) {
	h := e.Get[Health]()
	h.HP++
	e.Set(h) // owner: ok
}
```

## testdata/src/ai/ai.go

```go
package ai

import (
	"combat"
	"ecs"
)

func Hit(e ecs.Entity) {
	h := e.Get[combat.Health]()
	h.HP--
	e.Set(h)                  // want `Entity.Set writes Health, owned by package combat`
	e.Set[combat.Health](h)   // want `Entity.Set writes Health, owned by package combat`
	e.Remove[combat.Health]() // want `Entity.Remove writes Health, owned by package combat`
}

func Forgot(e ecs.Entity) {
	h := e.Get[combat.Health]() // reading is fine
	h.HP--                      // no Set: unusedwrite should flag this
}
```

## go.mod

```text
module ownercheck

go 1.27.4

require golang.org/x/tools v0.52.0

require (
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
)
```

## Type-instance probe (bench/instprobe/main_test.go)

```go
package instprobe

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

const src = `package p
type Entity struct{}
func (e Entity) Set[T any](c T) {}
func (e Entity) Get[T any]() (z T) { return }
type Health struct{ HP int }
func f(e Entity) {
	e.Set(Health{HP: 1})
	e.Set[Health](Health{})
	h := e.Get[Health]()
	h.HP++
	e.Set(h)
}`

func TestInstances(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Instances: map[*ast.Ident]types.Instance{}, Uses: map[*ast.Ident]types.Object{}}
	conf := types.Config{GoVersion: "go1.27"}
	if _, err := conf.Check("p", fset, []*ast.File{f}, info); err != nil {
		t.Fatal(err)
	}
	for id, inst := range info.Instances {
		named, _ := inst.TypeArgs.At(0).(*types.Named)
		pkg := ""
		if named != nil {
			pkg = named.Obj().Pkg().Path()
		}
		t.Logf("%s %s TypeArg=%s pkg=%s", fset.Position(id.Pos()), id.Name, inst.TypeArgs.At(0), pkg)
	}
}
```
