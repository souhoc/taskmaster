package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

var (
	typeName = flag.String("type", "", "Type with RPC mwthods")
)

func Usage() {
	fmt.Fprintln(os.Stderr, "Usage: go run main.go -type <type> [directory]")
	flag.PrintDefaults()
}

type Method struct {
	Name     string
	Constant string
}

func main() {
	flag.Usage = Usage
	flag.Parse()

	if *typeName == "" {
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()
	if len(args) == 0 {
		args = []string{"."}
	}
	dirPath := args[0]

	// Traverse the directory and parse each Go file
	var methods []Method
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Parse only Go files
		if filepath.Ext(path) != ".go" {
			return nil
		}

		// Parse the Go file
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil // Skip files that can't be parsed
		}

		// Inspect the AST to find methods of the specified type
		ast.Inspect(node, func(n ast.Node) bool {
			// Look for function declarations with a receiver of the specified type
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				return true
			}
			// Check if the receiver is of the specified type
			field := fn.Recv.List[0]
			var ident *ast.Ident
			if starExpr, ok := field.Type.(*ast.StarExpr); ok {
				ident = starExpr.X.(*ast.Ident)
			} else {
				ident = field.Type.(*ast.Ident)
			}
			if !ident.IsExported() || ident.Name != *typeName {
				return true
			}

			if !isRPCMethod(fn) {
				return true
			}

			// Found a method for the specified type
			methodName := fmt.Sprintf("%s.%s", ident, fn.Name)
			constantName := fmt.Sprintf("%s%s", ident, fn.Name)
			methods = append(methods, Method{Name: methodName, Constant: constantName})

			return true
		})

		return nil
	})

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error traversing directory:", err)
		os.Exit(1)
	}

	// Check if any methods were found
	if len(methods) == 0 {
		fmt.Fprintln(os.Stderr, "No methods compatible with RPC found for type:", typeName)
		os.Exit(1)
	}

	// Define the template for generating the constants
	const tmpl = `package taskmaster

// WARNING: This file is auto-generated. Do not edit manually.
// RPC method name constants
const (
{{- range .}}
    {{.Constant}} = "{{.Name}}"
{{- end}}
)
`

	// Create a template
	t := template.Must(template.New("constants").Parse(tmpl))

	var b bytes.Buffer
	if err := t.Execute(&b, methods); err != nil {
		fmt.Fprintf(os.Stderr, "Error executing template: %v", err)
		os.Exit(1)
	}

	src, err := format.Source(b.Bytes())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid Go generated: %v\n", err)
		os.Exit(1)
	}
	outputName := fmt.Sprintf("%s_const.go", strings.ToLower(*typeName))
	outputName = filepath.Join(dirPath, outputName)
	err = os.WriteFile(outputName, src, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "writing output: %v", err)
	}
}

func isRPCMethod(n *ast.FuncDecl) bool {
	// Check if the method is exported
	if !n.Name.IsExported() {
		return false
	}

	// Check if the method has exactly two arguments
	if n.Type.Params == nil || len(n.Type.Params.List) != 2 {
		return false
	}

	// Check if the second argument is a pointer
	_, isPtr := n.Type.Params.List[1].Type.(*ast.StarExpr)
	if !isPtr {
		return false
	}

	// Check if the method has a return type of error
	if n.Type.Results == nil || len(n.Type.Results.List) != 1 {
		return false
	}
	if ident, ok := n.Type.Results.List[0].Type.(*ast.Ident); !ok || ident.Name != "error" {
		return false
	}

	return true
}
