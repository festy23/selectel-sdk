package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// methodEvidence records declaration presence, not reachability or remote semantics.
func methodEvidence(root, dir string, bindings map[string]binding) error {
	packages := map[string]bool{}
	for _, b := range bindings {
		parts := strings.Split(b.GoMethod, ".")
		if len(parts) != 3 {
			return fmt.Errorf("invalid bound Go method %s", b.GoMethod)
		}
		packages[parts[0]] = true
	}
	methods := map[string]map[string]string{}
	for pkg := range packages {
		files, err := filepath.Glob(filepath.Join(root, pkg, "*.go"))
		if err != nil {
			return err
		}
		for _, file := range files {
			if strings.HasSuffix(file, "_test.go") {
				continue
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			parsed, err := parser.ParseFile(token.NewFileSet(), file, raw, 0)
			if err != nil {
				return err
			}
			for _, decl := range parsed.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || !fn.Name.IsExported() {
					continue
				}
				expr := fn.Recv.List[0].Type
				if star, ok := expr.(*ast.StarExpr); ok {
					expr = star.X
				}
				if index, ok := expr.(*ast.IndexExpr); ok {
					expr = index.X
				}
				if index, ok := expr.(*ast.IndexListExpr); ok {
					expr = index.X
				}
				recv, ok := expr.(*ast.Ident)
				if !ok || !recv.IsExported() {
					continue
				}
				key := pkg + "." + recv.Name + "." + fn.Name.Name
				rel, err := filepath.Rel(root, file)
				if err != nil {
					return err
				}
				if _, ok := methods[key]; ok {
					return fmt.Errorf("ambiguous method declaration %s", key)
				}
				methods[key] = map[string]string{"file": filepath.ToSlash(rel), "revision": hash(raw)}
			}
		}
	}
	return writeJSON(filepath.Join(dir, "contracts", "public-methods.json"), methods)
}
