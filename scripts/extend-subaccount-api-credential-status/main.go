// Command extend-subaccount-api-credential-status adds the handwritten lifecycle
// receipt field to Upjet's generated status type. It is intentionally narrow and
// fails if the generated target changes unexpectedly.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) != 1 {
		panic("usage: extend-subaccount-api-credential-status <generated-types.go>")
	}
	path := args[0]
	src, err := os.ReadFile(path)
	must(err)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments)
	must(err)
	found := false
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "SubaccountApiCredentialStatus" {
				continue
			}
			st, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				panic("SubaccountApiCredentialStatus is not a struct")
			}
			found = true
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					if name.Name == "Lifecycle" {
						return
					}
				}
			}
			st.Fields.List = append(st.Fields.List, &ast.Field{
				Names: []*ast.Ident{ast.NewIdent("Lifecycle")},
				Type:  &ast.StarExpr{X: ast.NewIdent("SubaccountApiCredentialLifecycleStatus")},
				Tag:   &ast.BasicLit{Kind: token.STRING, Value: "`json:\"lifecycle,omitempty\"`"},
			})
		}
	}
	if !found {
		panic("generated SubaccountApiCredentialStatus type not found")
	}
	var out bytes.Buffer
	must(format.Node(&out, fset, file))
	must(os.WriteFile(path, out.Bytes(), 0o644))
}

func must(err error) {
	if err != nil {
		panic(fmt.Errorf("extend generated lifecycle status: %w", err))
	}
}
