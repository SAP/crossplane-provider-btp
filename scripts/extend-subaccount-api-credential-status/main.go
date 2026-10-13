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
	out, err := extend(src, path)
	must(err)
	must(os.WriteFile(path, out, 0o644))
}

func extend(src []byte, filename string) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, err
	}
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
				return nil, fmt.Errorf("SubaccountApiCredentialStatus is not a struct")
			}
			found = true
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					if name.Name == "Lifecycle" {
						return src, nil
					}
				}
			}
			// AST nodes must carry source positions. Positionless nodes sort ahead
			// of trailing Kubebuilder comments when go/format prints the file,
			// accidentally attaching resource markers to the new struct field.
			pos := st.Fields.Closing - 1
			st.Fields.List = append(st.Fields.List, &ast.Field{
				Names: []*ast.Ident{{NamePos: pos, Name: "Lifecycle"}},
				Type:  &ast.StarExpr{Star: pos, X: &ast.Ident{NamePos: pos, Name: "SubaccountApiCredentialLifecycleStatus"}},
				Tag:   &ast.BasicLit{ValuePos: pos, Kind: token.STRING, Value: "`json:\"lifecycle,omitempty\"`"},
			})
		}
	}
	if !found {
		return nil, fmt.Errorf("generated SubaccountApiCredentialStatus type not found")
	}
	var out bytes.Buffer
	if err := format.Node(&out, fset, file); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func must(err error) {
	if err != nil {
		panic(fmt.Errorf("extend generated lifecycle status: %w", err))
	}
}
