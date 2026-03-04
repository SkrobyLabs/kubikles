// cmd/gen-dispatcher/main.go generates a switch-based method dispatcher
// for the App struct, replacing reflection-based dispatch to enable
// Go linker dead-code elimination (DCE).
//
// Usage: go run cmd/gen-dispatcher/main.go
//
// Scans all *.go files in the project root for exported methods on *App,
// and generates dispatch_gen.go implementing the server.MethodCaller interface.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// methodInfo holds parsed information about an App method.
type methodInfo struct {
	Name       string
	Params     []paramInfo
	Results    []resultInfo
	HasError   bool // last return is error
	ResultOnly bool // returns only error (no data return)
}

// paramInfo holds information about a method parameter.
type paramInfo struct {
	Name    string
	TypeStr string
}

// resultInfo holds information about a method return value.
type resultInfo struct {
	TypeStr string
}

func main() {
	// Find the project root (where go.mod is)
	rootDir := "."
	if len(os.Args) > 1 {
		rootDir = os.Args[1]
	}

	// Parse all Go files in the root directory
	fset := token.NewFileSet()
	methods := parseAppMethods(fset, rootDir)

	if len(methods) == 0 {
		log.Fatal("No exported App methods found")
	}

	log.Printf("Found %d unique exported App methods", len(methods))

	// Generate the dispatcher
	code := generateDispatcher(methods)

	// Format the code
	formatted, err := format.Source(code)
	if err != nil {
		// Write unformatted for debugging
		_ = os.WriteFile(filepath.Join(rootDir, "dispatch_gen.go"), code, 0600)
		log.Fatalf("Failed to format generated code: %v\nUnformatted code written to dispatch_gen.go for debugging", err)
	}

	// Write the file
	outPath := filepath.Join(rootDir, "dispatch_gen.go")
	if err := os.WriteFile(outPath, formatted, 0600); err != nil {
		log.Fatalf("Failed to write %s: %v", outPath, err)
	}

	log.Printf("Generated %s with %d method cases", outPath, len(methods))
}

// parseAppMethods scans Go files for exported methods on *App.
func parseAppMethods(fset *token.FileSet, dir string) []methodInfo {
	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Fatalf("Failed to read directory %s: %v", dir, err)
	}

	// Track seen method names to deduplicate (helm vs !helm stubs have same methods)
	seen := make(map[string]bool)
	var methods []methodInfo

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		// Skip test files and the generated file itself
		if strings.HasSuffix(name, "_test.go") || name == "dispatch_gen.go" {
			continue
		}

		filePath := filepath.Join(dir, name)
		file, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
		if err != nil {
			log.Printf("Warning: failed to parse %s: %v", filePath, err)
			continue
		}

		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Recv == nil {
				continue
			}

			// Check it's a method on *App
			if !isAppReceiver(funcDecl.Recv) {
				continue
			}

			methodName := funcDecl.Name.Name
			// Only exported methods
			if !unicode.IsUpper(rune(methodName[0])) {
				continue
			}

			// Skip duplicates (e.g., from helm and !helm builds)
			if seen[methodName] {
				continue
			}
			seen[methodName] = true

			mi := extractMethodInfo(fset, funcDecl)
			methods = append(methods, mi)
		}
	}

	// Sort by method name for deterministic output
	sort.Slice(methods, func(i, j int) bool {
		return methods[i].Name < methods[j].Name
	})

	return methods
}

// isAppReceiver checks if the receiver is *App.
func isAppReceiver(fieldList *ast.FieldList) bool {
	if fieldList == nil || len(fieldList.List) != 1 {
		return false
	}
	recv := fieldList.List[0]
	starExpr, ok := recv.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := starExpr.X.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == "App"
}

// extractMethodInfo extracts parameter and return type information from a method declaration.
func extractMethodInfo(fset *token.FileSet, funcDecl *ast.FuncDecl) methodInfo {
	mi := methodInfo{Name: funcDecl.Name.Name}

	// Extract parameters
	if funcDecl.Type.Params != nil {
		paramIdx := 0
		for _, field := range funcDecl.Type.Params.List {
			typeStr := exprToString(fset, field.Type)
			if len(field.Names) == 0 {
				// Unnamed parameter (e.g., in stubs)
				mi.Params = append(mi.Params, paramInfo{
					Name:    fmt.Sprintf("arg%d", paramIdx),
					TypeStr: typeStr,
				})
				paramIdx++
			} else {
				for _, name := range field.Names {
					pName := name.Name
					if pName == "_" {
						pName = fmt.Sprintf("arg%d", paramIdx)
					}
					mi.Params = append(mi.Params, paramInfo{
						Name:    pName,
						TypeStr: typeStr,
					})
					paramIdx++
				}
			}
		}
	}

	// Extract results
	if funcDecl.Type.Results != nil {
		for _, field := range funcDecl.Type.Results.List {
			typeStr := exprToString(fset, field.Type)
			count := len(field.Names)
			if count == 0 {
				count = 1
			}
			for range count {
				mi.Results = append(mi.Results, resultInfo{TypeStr: typeStr})
			}
		}
	}

	// Check if last result is error
	if len(mi.Results) > 0 && mi.Results[len(mi.Results)-1].TypeStr == "error" {
		mi.HasError = true
		if len(mi.Results) == 1 {
			mi.ResultOnly = true
		}
	}

	return mi
}

// exprToString converts an AST expression to its string representation.
func exprToString(_ *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	writeExpr(&buf, expr)
	return buf.String()
}

// writeExpr writes the string representation of an AST expression.
func writeExpr(buf *bytes.Buffer, expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.Ident:
		buf.WriteString(e.Name)
	case *ast.SelectorExpr:
		writeExpr(buf, e.X)
		buf.WriteByte('.')
		buf.WriteString(e.Sel.Name)
	case *ast.StarExpr:
		buf.WriteByte('*')
		writeExpr(buf, e.X)
	case *ast.ArrayType:
		buf.WriteString("[]")
		writeExpr(buf, e.Elt)
	case *ast.MapType:
		buf.WriteString("map[")
		writeExpr(buf, e.Key)
		buf.WriteByte(']')
		writeExpr(buf, e.Value)
	case *ast.InterfaceType:
		buf.WriteString("interface{}")
	case *ast.Ellipsis:
		buf.WriteString("...")
		writeExpr(buf, e.Elt)
	default:
		buf.WriteString("interface{}")
	}
}

// collectPackageRefs scans all type strings in methods and returns the set of
// package prefixes used (e.g., "helm", "k8s", "terminal").
func collectPackageRefs(methods []methodInfo) map[string]bool {
	refs := make(map[string]bool)
	for _, m := range methods {
		for _, p := range m.Params {
			extractPkgRefs(p.TypeStr, refs)
		}
	}
	return refs
}

// extractPkgRefs extracts package references from a type string.
func extractPkgRefs(typeStr string, refs map[string]bool) {
	// Strip pointer, slice, map prefixes
	s := typeStr
	for {
		if strings.HasPrefix(s, "*") {
			s = s[1:]
		} else if strings.HasPrefix(s, "[]") {
			s = s[2:]
		} else if strings.HasPrefix(s, "map[") {
			// Extract both key and value types
			depth := 1
			i := 4
			for i < len(s) && depth > 0 {
				if s[i] == '[' {
					depth++
				} else if s[i] == ']' {
					depth--
				}
				i++
			}
			if i < len(s) {
				extractPkgRefs(s[4:i-1], refs) // key
				extractPkgRefs(s[i:], refs)    // value
			}
			return
		} else {
			break
		}
	}
	// Check for pkg.Type pattern
	if idx := strings.Index(s, "."); idx > 0 {
		refs[s[:idx]] = true
	}
}

// knownPackages maps short package names to their import paths.
var knownPackages = map[string]string{
	"helm":          "kubikles/pkg/helm",
	"k8s":           "kubikles/pkg/k8s",
	"terminal":      "kubikles/pkg/terminal",
	"events":        "kubikles/pkg/events",
	"tools":         "kubikles/pkg/tools",
	"issuedetector": "kubikles/pkg/issuedetector",
}

// generateDispatcher produces the Go source code for the dispatch file.
func generateDispatcher(methods []methodInfo) []byte {
	var buf bytes.Buffer

	// Collect package references
	pkgRefs := collectPackageRefs(methods)

	buf.WriteString("// Code generated by cmd/gen-dispatcher. DO NOT EDIT.\n\n")
	buf.WriteString("package main\n\n")
	buf.WriteString("import (\n")
	buf.WriteString("\t\"encoding/json\"\n")
	buf.WriteString("\t\"fmt\"\n")
	buf.WriteString("\n")
	buf.WriteString("\t\"kubikles/pkg/server\"\n")

	// Add imports for referenced packages
	importedPkgs := make([]string, 0, len(pkgRefs))
	for pkg := range pkgRefs {
		if importPath, ok := knownPackages[pkg]; ok {
			importedPkgs = append(importedPkgs, importPath)
		}
	}
	sort.Strings(importedPkgs)
	for _, imp := range importedPkgs {
		buf.WriteString(fmt.Sprintf("\t%q\n", imp))
	}

	buf.WriteString(")\n\n")

	buf.WriteString(`// AppMethodCaller implements server.MethodCaller using a direct switch dispatch,
// avoiding reflect.MethodByName to enable Go linker dead-code elimination (DCE).
type AppMethodCaller struct {
	app *App
}

// Compile-time interface check.
var _ server.MethodCaller = (*AppMethodCaller)(nil)

// NewAppMethodCaller creates a new switch-based method caller wrapping the App.
func NewAppMethodCaller(app *App) *AppMethodCaller {
	return &AppMethodCaller{app: app}
}

// unmarshalArg unmarshals a JSON argument into a value of type T.
// Returns the zero value if the argument index is out of range or nil.
func unmarshalArg[T any](args []json.RawMessage, index int) (T, error) {
	var v T
	if index >= len(args) || args[index] == nil {
		return v, nil
	}
	if err := json.Unmarshal(args[index], &v); err != nil {
		return v, fmt.Errorf("argument %d: %w", index, err)
	}
	return v, nil
}

// CallMethod dispatches a method call by name with JSON-encoded arguments.
func (c *AppMethodCaller) CallMethod(methodName string, args []json.RawMessage) (interface{}, error) {
	switch methodName {
`)

	for _, m := range methods {
		// Skip methods with variadic params - they can't be dispatched via JSON args
		hasVariadic := false
		for _, p := range m.Params {
			if strings.HasPrefix(p.TypeStr, "...") {
				hasVariadic = true
				break
			}
		}
		if hasVariadic {
			continue
		}

		buf.WriteString(fmt.Sprintf("\tcase %q:\n", m.Name))

		// Unmarshal arguments
		for i, p := range m.Params {
			buf.WriteString(fmt.Sprintf("\t\tp%d, err := unmarshalArg[%s](args, %d)\n", i, p.TypeStr, i))
			buf.WriteString("\t\tif err != nil {\n")
			buf.WriteString("\t\t\treturn nil, err\n")
			buf.WriteString("\t\t}\n")
		}

		// Build the call
		callArgs := make([]string, len(m.Params))
		for i := range m.Params {
			callArgs[i] = fmt.Sprintf("p%d", i)
		}
		callStr := fmt.Sprintf("c.app.%s(%s)", m.Name, strings.Join(callArgs, ", "))

		// Handle different return patterns
		switch {
		case len(m.Results) == 0:
			// No return values
			buf.WriteString(fmt.Sprintf("\t\t%s\n", callStr))
			buf.WriteString("\t\treturn nil, nil\n")

		case m.ResultOnly:
			// Returns only error
			buf.WriteString(fmt.Sprintf("\t\treturn nil, %s\n", callStr))

		case m.HasError:
			// Returns (value, error)
			buf.WriteString(fmt.Sprintf("\t\tresult, err := %s\n", callStr))
			buf.WriteString("\t\treturn result, err\n")

		case len(m.Results) == 1:
			// Returns single value, no error
			buf.WriteString(fmt.Sprintf("\t\treturn %s, nil\n", callStr))

		default:
			// Multiple returns without error (unlikely but handle it)
			retVars := make([]string, len(m.Results))
			for i := range m.Results {
				retVars[i] = fmt.Sprintf("r%d", i)
			}
			buf.WriteString(fmt.Sprintf("\t\t%s := %s\n", strings.Join(retVars, ", "), callStr))
			retSlice := make([]string, len(m.Results))
			for i := range m.Results {
				retSlice[i] = retVars[i]
			}
			buf.WriteString(fmt.Sprintf("\t\treturn []interface{}{%s}, nil\n", strings.Join(retSlice, ", ")))
		}
	}

	buf.WriteString(`	default:
		return nil, fmt.Errorf("method %q not found", methodName)
	}
}
`)

	return buf.Bytes()
}
