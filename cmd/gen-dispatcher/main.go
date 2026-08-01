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
	Excluded   bool
	Sources    []string
	Imports    map[string]string // qualifier to import path used by the signature
}

// paramInfo holds information about a method parameter.
type paramInfo struct {
	Name           string
	TypeStr        string
	TrustedContext bool
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
	methods, err := parseAppMethods(fset, rootDir)
	if err != nil {
		log.Fatal(err)
	}

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
func parseAppMethods(fset *token.FileSet, dir string) ([]methodInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read directory %s: %w", dir, err)
	}

	groups := make(map[string][]methodInfo)

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
			return nil, fmt.Errorf("parse %s: %w", filePath, err)
		}

		fileImports := importsByQualifier(file)
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
			if !ast.IsExported(methodName) {
				continue
			}

			mi := extractMethodInfo(fset, funcDecl)
			mi.Imports = signatureImports(mi, fileImports)
			classifyTrustedContexts(&mi)
			excluded, err := parseDispatchDirective(funcDecl.Doc)
			if err != nil {
				return nil, fmt.Errorf("%s in %s: %w", methodName, name, err)
			}
			mi.Excluded = excluded
			mi.Sources = []string{name}
			groups[methodName] = append(groups[methodName], mi)
		}
	}

	groupNames := make([]string, 0, len(groups))
	for name := range groups {
		groupNames = append(groupNames, name)
	}
	sort.Strings(groupNames)

	methods := make([]methodInfo, 0, len(groups))
	for _, name := range groupNames {
		variants := groups[name]
		base := variants[0]
		for _, variant := range variants[1:] {
			if !sameMethodSignature(base, variant) || base.Excluded != variant.Excluded {
				return nil, fmt.Errorf("method %s differs across %s and %s", name, base.Sources[0], variant.Sources[0])
			}
			if qualifier, ok := conflictingImportQualifier(base, variant); ok {
				return nil, fmt.Errorf("method %s has conflicting import path for qualifier %q across %s and %s", name, qualifier, base.Sources[0], variant.Sources[0])
			}
			base.Sources = append(base.Sources, variant.Sources[0])
		}
		sort.Strings(base.Sources)
		if err := validateMethod(base); err != nil {
			return nil, fmt.Errorf("method %s in %s: %w", name, strings.Join(base.Sources, ", "), err)
		}
		methods = append(methods, base)
	}

	if err := validateGeneratedImports(methods); err != nil {
		return nil, err
	}

	return methods, nil
}

func parseDispatchDirective(doc *ast.CommentGroup) (bool, error) {
	if doc == nil {
		return false, nil
	}
	var directive string
	for _, comment := range doc.List {
		const namespace = "kubikles:dispatch"
		if !strings.HasPrefix(comment.Text, "//"+namespace) {
			continue
		}
		text := strings.TrimPrefix(comment.Text, "//")
		suffix := strings.TrimPrefix(text, namespace)
		if suffix != "" && !unicode.IsSpace([]rune(suffix)[0]) {
			continue
		}

		value := strings.TrimPrefix(text, namespace+" ")
		if value == text || value == "" || strings.ContainsFunc(value, unicode.IsSpace) {
			return false, fmt.Errorf("invalid dispatch directive %q", text)
		}
		if directive != "" && directive != value {
			return false, fmt.Errorf("conflicting dispatch directives")
		}
		directive = value
	}
	if directive == "" {
		return false, nil
	}
	if directive != "exclude" {
		return false, fmt.Errorf("unknown dispatch directive %q", directive)
	}
	return true, nil
}

func sameMethodSignature(a, b methodInfo) bool {
	if len(a.Params) != len(b.Params) || len(a.Results) != len(b.Results) {
		return false
	}
	for i := range a.Params {
		if a.Params[i].TrustedContext && b.Params[i].TrustedContext {
			continue
		}
		if a.Params[i].TypeStr != b.Params[i].TypeStr || a.Params[i].TrustedContext != b.Params[i].TrustedContext {
			return false
		}
	}
	for i := range a.Results {
		if a.Results[i].TypeStr != b.Results[i].TypeStr {
			return false
		}
	}
	return true
}

func validateMethod(m methodInfo) error {
	contextCount := 0
	for i, param := range m.Params {
		if param.TrustedContext {
			contextCount++
			if i != 0 {
				return fmt.Errorf("AuthenticatedCallContext must be the first parameter")
			}
		}
		if strings.HasPrefix(param.TypeStr, "...") && !m.Excluded {
			return fmt.Errorf("variadic method must use //kubikles:dispatch exclude")
		}
	}
	if contextCount > 1 {
		return fmt.Errorf("AuthenticatedCallContext may appear at most once")
	}
	if !m.Excluded {
		for _, typeStr := range methodTypeStrings(m) {
			for qualifier := range packageRefs(typeStr) {
				if _, ok := m.Imports[qualifier]; !ok {
					return fmt.Errorf("unresolved package qualifier %q", qualifier)
				}
			}
		}
	}
	return nil
}

func classifyTrustedContexts(m *methodInfo) {
	for i := range m.Params {
		qualifier, ok := exactSelectorQualifier(m.Params[i].TypeStr, "AuthenticatedCallContext")
		m.Params[i].TrustedContext = ok && m.Imports[qualifier] == "kubikles/pkg/agent"
	}
}

func exactSelectorQualifier(typeStr, selectorName string) (string, bool) {
	expr, err := parser.ParseExpr(typeStr)
	if err != nil {
		return "", false
	}
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != selectorName {
		return "", false
	}
	qualifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return "", false
	}
	return qualifier.Name, true
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
func exprToString(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, expr); err != nil {
		return "<invalid type>"
	}
	return buf.String()
}

var requiredGeneratedImports = map[string]string{
	"agent":  "kubikles/pkg/agent",
	"fmt":    "fmt",
	"json":   "encoding/json",
	"server": "kubikles/pkg/server",
}

// collectGeneratedImports returns one qualifier-to-path table containing the
// dispatcher's required imports and eligible parameter type references. Result
// types are inferred from App calls, so importing their packages would be unused.
func collectGeneratedImports(methods []methodInfo) map[string]string {
	refs := make(map[string]string, len(requiredGeneratedImports))
	for qualifier, path := range requiredGeneratedImports {
		refs[qualifier] = path
	}
	for _, m := range methods {
		if m.Excluded {
			continue
		}
		for _, param := range m.Params {
			if param.TrustedContext {
				continue
			}
			for qualifier := range packageRefs(param.TypeStr) {
				refs[qualifier] = m.Imports[qualifier]
			}
		}
	}
	return refs
}

type importBinding struct {
	qualifier string
	path      string
	owner     string
}

func validateGeneratedImports(methods []methodInfo) error {
	byQualifier := make(map[string]importBinding, len(requiredGeneratedImports))
	byPath := make(map[string]importBinding, len(requiredGeneratedImports))
	for _, qualifier := range sortedImportQualifiers(requiredGeneratedImports) {
		binding := importBinding{qualifier: qualifier, path: requiredGeneratedImports[qualifier], owner: "reserved generated binding"}
		byQualifier[binding.qualifier] = binding
		byPath[binding.path] = binding
	}

	for _, method := range methods {
		if method.Excluded {
			continue
		}
		qualifierSet := make(map[string]bool)
		for _, param := range method.Params {
			if param.TrustedContext {
				continue
			}
			for qualifier := range packageRefs(param.TypeStr) {
				qualifierSet[qualifier] = true
			}
		}
		qualifiers := make([]string, 0, len(qualifierSet))
		for qualifier := range qualifierSet {
			qualifiers = append(qualifiers, qualifier)
		}
		sort.Strings(qualifiers)
		for _, qualifier := range qualifiers {
			binding := importBinding{
				qualifier: qualifier,
				path:      method.Imports[qualifier],
				owner:     fmt.Sprintf("method %s in %s", method.Name, strings.Join(method.Sources, ", ")),
			}
			if previous, ok := byQualifier[qualifier]; ok {
				if previous.path != binding.path {
					return fmt.Errorf("generated import qualifier %q maps to both %q (%s) and %q (%s)", qualifier, previous.path, previous.owner, binding.path, binding.owner)
				}
				continue
			}
			if previous, ok := byPath[binding.path]; ok && previous.qualifier != qualifier {
				return fmt.Errorf("generated import path %q uses both qualifier %q (%s) and qualifier %q (%s)", binding.path, previous.qualifier, previous.owner, qualifier, binding.owner)
			}
			byQualifier[qualifier] = binding
			byPath[binding.path] = binding
		}
	}
	return nil
}

func sortedImportQualifiers(imports map[string]string) []string {
	qualifiers := make([]string, 0, len(imports))
	for qualifier := range imports {
		qualifiers = append(qualifiers, qualifier)
	}
	sort.Strings(qualifiers)
	return qualifiers
}

// importsByQualifier records the import path selected by each qualifier in a
// source file. Explicit aliases are authoritative; known package paths cover
// ordinary imports whose declared package name is not their path base.
func importsByQualifier(file *ast.File) map[string]string {
	imports := make(map[string]string)
	for _, spec := range file.Imports {
		path := strings.Trim(spec.Path.Value, `"`)
		if spec.Name != nil {
			if name := spec.Name.Name; name != "_" && name != "." {
				imports[name] = path
			}
			continue
		}
		imports[filepath.Base(path)] = path
		for qualifier, knownPath := range knownPackages {
			if knownPath == path {
				imports[qualifier] = path
			}
		}
	}
	return imports
}

func signatureImports(m methodInfo, fileImports map[string]string) map[string]string {
	imports := make(map[string]string)
	for _, typeStr := range methodTypeStrings(m) {
		for qualifier := range packageRefs(typeStr) {
			if path, ok := fileImports[qualifier]; ok {
				imports[qualifier] = path
			}
		}
	}
	return imports
}

func conflictingImportQualifier(a, b methodInfo) (string, bool) {
	qualifierSet := make(map[string]bool)
	for _, param := range a.Params {
		if param.TrustedContext {
			continue
		}
		for qualifier := range packageRefs(param.TypeStr) {
			qualifierSet[qualifier] = true
		}
	}
	for _, result := range a.Results {
		for qualifier := range packageRefs(result.TypeStr) {
			qualifierSet[qualifier] = true
		}
	}
	qualifiers := make([]string, 0, len(qualifierSet))
	for qualifier := range qualifierSet {
		qualifiers = append(qualifiers, qualifier)
	}
	sort.Strings(qualifiers)
	for _, qualifier := range qualifiers {
		if a.Imports[qualifier] != b.Imports[qualifier] {
			return qualifier, true
		}
	}
	return "", false
}

func methodTypeStrings(m methodInfo) []string {
	types := make([]string, 0, len(m.Params)+len(m.Results))
	for _, param := range m.Params {
		types = append(types, param.TypeStr)
	}
	for _, result := range m.Results {
		types = append(types, result.TypeStr)
	}
	return types
}

// packageRefs extracts package qualifiers from any valid Go type expression.
func packageRefs(typeStr string) map[string]bool {
	refs := make(map[string]bool)
	expr, err := parser.ParseExpr(typeStr)
	if err != nil {
		return refs
	}
	ast.Inspect(expr, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if ident, ok := selector.X.(*ast.Ident); ok {
			refs[ident.Name] = true
		}
		return true
	})
	return refs
}

// knownPackages maps short package names to their import paths.
var knownPackages = map[string]string{
	"agent":                   "kubikles/pkg/agent",
	"helm":                    "kubikles/pkg/helm",
	"k8s":                     "kubikles/pkg/k8s",
	"terminal":                "kubikles/pkg/terminal",
	"events":                  "kubikles/pkg/events",
	"tools":                   "kubikles/pkg/tools",
	"issuedetector":           "kubikles/pkg/issuedetector",
	"admissionregistrationv1": "k8s.io/api/admissionregistration/v1",
	"apiextensionsv1":         "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1",
	"appsv1":                  "k8s.io/api/apps/v1",
	"autoscalingv2":           "k8s.io/api/autoscaling/v2",
	"batchv1":                 "k8s.io/api/batch/v1",
	"coordinationv1":          "k8s.io/api/coordination/v1",
	"corev1":                  "k8s.io/api/core/v1",
	"discoveryv1":             "k8s.io/api/discovery/v1",
	"networkingv1":            "k8s.io/api/networking/v1",
	"policyv1":                "k8s.io/api/policy/v1",
	"rbacv1":                  "k8s.io/api/rbac/v1",
	"schedulingv1":            "k8s.io/api/scheduling/v1",
	"storagev1":               "k8s.io/api/storage/v1",
	"v1":                      "k8s.io/api/core/v1",
}

// generateDispatcher produces the Go source code for the dispatch file.
func generateDispatcher(methods []methodInfo) []byte {
	var buf bytes.Buffer

	imports := collectGeneratedImports(methods)

	buf.WriteString("// Code generated by cmd/gen-dispatcher. DO NOT EDIT.\n\n")
	for _, method := range methods {
		if method.Excluded {
			buf.WriteString(fmt.Sprintf("// kubikles:dispatch exclude %s (%s)\n", method.Name, strings.Join(method.Sources, ", ")))
		}
	}
	if len(methods) > 0 {
		buf.WriteByte('\n')
	}
	buf.WriteString("package main\n\n")
	buf.WriteString("import (\n")
	for _, qualifier := range sortedImportQualifiers(imports) {
		buf.WriteString(fmt.Sprintf("\t%s %q\n", qualifier, imports[qualifier]))
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
func (c *AppMethodCaller) CallMethod(callContext agent.AuthenticatedCallContext, methodName string, args []json.RawMessage) (interface{}, error) {
	switch methodName {
`)

	for _, m := range methods {
		if m.Excluded {
			continue
		}

		buf.WriteString(fmt.Sprintf("\tcase %q:\n", m.Name))

		// Unmarshal arguments
		paramOffset := 0
		if len(m.Params) > 0 && m.Params[0].TrustedContext {
			paramOffset = 1
		}
		for i := paramOffset; i < len(m.Params); i++ {
			p := m.Params[i]
			// Trusted contexts are injected, not JSON parameters; locals are named
			// by JSON position so generated calls remain literally callContext,p0,... .
			local := i - paramOffset
			buf.WriteString(fmt.Sprintf("\t\tp%d, err := unmarshalArg[%s](args, %d)\n", local, p.TypeStr, local))
			buf.WriteString("\t\tif err != nil {\n")
			buf.WriteString("\t\t\treturn nil, err\n")
			buf.WriteString("\t\t}\n")
		}

		// Build the call
		callArgs := make([]string, 0, len(m.Params))
		if paramOffset == 1 {
			callArgs = append(callArgs, "callContext")
		}
		for i := paramOffset; i < len(m.Params); i++ {
			callArgs = append(callArgs, fmt.Sprintf("p%d", i-paramOffset))
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
			// Returns one or more values followed by error.
			valueCount := len(m.Results) - 1
			retVars := make([]string, 0, len(m.Results))
			for i := 0; i < valueCount; i++ {
				retVars = append(retVars, fmt.Sprintf("r%d", i))
			}
			retVars = append(retVars, "err")
			buf.WriteString(fmt.Sprintf("\t\t%s := %s\n", strings.Join(retVars, ", "), callStr))
			buf.WriteString("\t\tif err != nil {\n")
			buf.WriteString("\t\t\treturn nil, err\n")
			buf.WriteString("\t\t}\n")
			if valueCount == 1 {
				buf.WriteString("\t\treturn r0, nil\n")
			} else {
				buf.WriteString(fmt.Sprintf("\t\treturn []interface{}{%s}, nil\n", strings.Join(retVars[:valueCount], ", ")))
			}

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
