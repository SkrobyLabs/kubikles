package main

import (
	"fmt"
	"go/format"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kubikles/pkg/agent"
)

func TestParseAppMethodsUsesUnicodeExportRules(t *testing.T) {
	methods, _ := fixtureMethods(t, map[string]string{"app.go": `package main
type App struct{}
func (*App) Éxported() {}
func (*App) éxcluded() {}
`})
	if len(methods) != 1 || methods[0].Name != "Éxported" {
		t.Fatalf("methods = %#v", methods)
	}
}

func TestGenerateDispatcherPreservesSourceImportAliases(t *testing.T) {
	methods, _ := fixtureMethods(t, map[string]string{
		"a.go": `//go:build one
package main
import appsv1 "k8s.io/api/apps/v1"
type App struct{}
func (*App) Deployments(value map[string][]*appsv1.Deployment) (map[string][]*appsv1.Deployment, error) { return value, nil }
`,
		"b.go": `//go:build !one
package main
import appsv1 "k8s.io/api/apps/v1"
func (*App) Deployments(value map[string][]*appsv1.Deployment) (map[string][]*appsv1.Deployment, error) { return value, nil }
`,
	})
	generated, err := format.Source(generateDispatcher(methods))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), `appsv1 "k8s.io/api/apps/v1"`) {
		t.Fatalf("source alias was lost:\n%s", generated)
	}
}

func TestParseAppMethodsRejectsConflictingSourceImportAliases(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.go", "//go:build one\npackage main\nimport appsv1 \"example.com/one\"\ntype App struct{}\nfunc (*App) Deployments(value appsv1.Deployment) {}\n")
	writeFixture(t, dir, "b.go", "//go:build !one\npackage main\nimport appsv1 \"example.com/two\"\nfunc (*App) Deployments(value appsv1.Deployment) {}\n")
	_, err := parseAppMethods(token.NewFileSet(), dir)
	if err == nil || !strings.Contains(err.Error(), "appsv1") || !strings.Contains(err.Error(), "a.go") || !strings.Contains(err.Error(), "b.go") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseAppMethodsRejectsGlobalGeneratedImportConflicts(t *testing.T) {
	tests := map[string]struct {
		files map[string]string
		wants []string
	}{
		"qualifier maps to different paths": {
			files: map[string]string{
				"a.go": "package main\nimport shared \"example.com/one\"\ntype App struct{}\nfunc (*App) Alpha(value shared.Value) {}\n",
				"b.go": "package main\nimport shared \"example.com/two\"\nfunc (*App) Beta(value shared.Value) {}\n",
			},
			wants: []string{`qualifier "shared"`, "example.com/one", "Alpha", "a.go", "example.com/two", "Beta", "b.go"},
		},
		"path maps to different qualifiers": {
			files: map[string]string{
				"a.go": "package main\nimport first \"example.com/shared\"\ntype App struct{}\nfunc (*App) Alpha(value first.Value) {}\n",
				"b.go": "package main\nimport second \"example.com/shared\"\nfunc (*App) Beta(value second.Value) {}\n",
			},
			wants: []string{`path "example.com/shared"`, `qualifier "first"`, "Alpha", "a.go", `qualifier "second"`, "Beta", "b.go"},
		},
	}
	for _, reserved := range []string{"agent", "server", "json", "fmt"} {
		tests["reserved "+reserved] = struct {
			files map[string]string
			wants []string
		}{
			files: map[string]string{
				"app.go": "package main\nimport " + reserved + " \"example.com/not-reserved\"\ntype App struct{}\nfunc (*App) Method(value " + reserved + ".Value) {}\n",
			},
			wants: []string{`qualifier "` + reserved + `"`, "reserved generated binding", "Method", "app.go"},
		}
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for filename, source := range test.files {
				writeFixture(t, dir, filename, source)
			}
			_, err := parseAppMethods(token.NewFileSet(), dir)
			if err == nil {
				t.Fatal("expected import conflict")
			}
			for _, want := range test.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
		})
	}
}

func TestGeneratedDispatcherCompilesWithImportsSharedAcrossMethods(t *testing.T) {
	methods, dir := fixtureMethods(t, map[string]string{
		"a.go": "package main\nimport shared \"dispatchfixture/shared\"\ntype App struct{}\nfunc (*App) Alpha(value shared.Value) shared.Value { return value }\n",
		"b.go": "package main\nimport shared \"dispatchfixture/shared\"\nfunc (*App) Beta(value shared.Value) shared.Value { return value }\n",
	})
	if err := os.MkdirAll(filepath.Join(dir, "shared"), 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(dir, "shared"), "value.go", "package shared\ntype Value struct { Name string }\n")
	generated, err := format.Source(generateDispatcher(methods))
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "dispatch_gen.go", string(generated))
	writeFixture(t, dir, "dispatch_test.go", `package main
import (
    "encoding/json"
    "testing"
    "kubikles/pkg/agent"
)
func TestSharedImportDispatch(t *testing.T) {
    caller := NewAppMethodCaller(&App{})
    for _, method := range []string{"Alpha", "Beta"} {
        result, err := caller.CallMethod(agent.AuthenticatedCallContext{}, method, []json.RawMessage{json.RawMessage("{\"Name\":\"value\"}")})
        if err != nil { t.Fatal(err) }
        if result == nil { t.Fatalf("%s returned nil", method) }
    }
}
`)
	root := repositoryRoot(t)
	writeFixture(t, dir, "go.mod", "module dispatchfixture\n\ngo 1.24\n\nrequire kubikles v0.0.0\n\nreplace kubikles => "+root+"\n")
	cmd := exec.Command("go", "test", "-mod=mod", ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated fixture did not compile or run: %v\n%s", err, output)
	}
}

func TestGeneratedDispatcherEmitsReservedImportsOnce(t *testing.T) {
	methods, dir := fixtureMethods(t, map[string]string{"app.go": `package main
import (
    "encoding/json"
    "fmt"
    "kubikles/pkg/agent"
    "kubikles/pkg/server"
)
type App struct{}
func (*App) ReservedImports(
    messages map[string][]json.RawMessage,
    stringers []fmt.Stringer,
    events map[string]*server.Event,
    identities []agent.BuildIdentity,
) {}
`})
	generated, err := format.Source(generateDispatcher(methods))
	if err != nil {
		t.Fatal(err)
	}
	for _, importPath := range []string{"encoding/json", "fmt", "kubikles/pkg/agent", "kubikles/pkg/server"} {
		if count := strings.Count(string(generated), `"`+importPath+`"`); count != 1 {
			t.Errorf("generated import %q count = %d, want 1:\n%s", importPath, count, generated)
		}
	}
	writeFixture(t, dir, "dispatch_gen.go", string(generated))
	root := repositoryRoot(t)
	writeFixture(t, dir, "go.mod", "module dispatchfixture\n\ngo 1.24\n\nrequire kubikles v0.0.0\n\nreplace kubikles => "+root+"\n")
	cmd := exec.Command("go", "test", "-mod=mod", ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated fixture did not compile: %v\n%s", err, output)
	}
}

func TestGeneratedDispatcherCompilesAndHandlesMultipleResultsBeforeError(t *testing.T) {
	methods, dir := fixtureMethods(t, map[string]string{"app.go": `package main
import (
    "errors"
    appsv1 "k8s.io/api/apps/v1"
)
type App struct{}
func (*App) Multi(value int, deployments map[string][]*appsv1.Deployment) (int, string, error) {
    if value < 0 { return 0, "", errors.New("failed") }
    return value, "ok", nil
}
`})
	generated, err := format.Source(generateDispatcher(methods))
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "dispatch_gen.go", string(generated))
	writeFixture(t, dir, "dispatch_test.go", `package main
import (
    "encoding/json"
    "testing"
    "kubikles/pkg/agent"
)
func TestGeneratedMulti(t *testing.T) {
    caller := NewAppMethodCaller(&App{})
    result, err := caller.CallMethod(agent.AuthenticatedCallContext{}, "Multi", []json.RawMessage{json.RawMessage("7"), json.RawMessage(`+"`{}`"+`)})
    if err != nil { t.Fatal(err) }
    values, ok := result.([]interface{})
    if !ok || len(values) != 2 || values[0] != 7 || values[1] != "ok" { t.Fatalf("result = %#v", result) }
    result, err = caller.CallMethod(agent.AuthenticatedCallContext{}, "Multi", []json.RawMessage{json.RawMessage("-1"), json.RawMessage(`+"`{}`"+`)})
    if result != nil || err == nil || err.Error() != "failed" { t.Fatalf("result, err = %#v, %v", result, err) }
}
`)
	root := repositoryRoot(t)
	writeFixture(t, dir, "go.mod", "module dispatchfixture\n\ngo 1.24\n\nrequire kubikles v0.0.0\n\nreplace kubikles => "+root+"\n")
	cmd := exec.Command("go", "test", "-mod=mod", ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated fixture did not compile or run: %v\n%s", err, output)
	}
}

func writeFixture(t *testing.T, dir, name, source string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
}

func fixtureMethods(t *testing.T, files map[string]string) ([]methodInfo, string) {
	t.Helper()
	dir := t.TempDir()
	for name, source := range files {
		writeFixture(t, dir, name, source)
	}
	methods, err := parseAppMethods(token.NewFileSet(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return methods, dir
}

func TestParseAppMethodsClassifiesIncludeAndExclude(t *testing.T) {
	methods, _ := fixtureMethods(t, map[string]string{"app.go": `package main
type App struct{}
func (*App) Include(value string) {}
//kubikles:dispatch exclude
func (*App) Exclude() {}
`})
	if len(methods) != 2 || methods[0].Name != "Exclude" || !methods[0].Excluded || methods[1].Excluded {
		t.Fatalf("classification = %#v", methods)
	}
	generated := string(generateDispatcher(methods))
	if strings.Contains(generated, `case "Exclude"`) || !strings.Contains(generated, `case "Include"`) || !strings.Contains(generated, "exclude Exclude (app.go)") {
		t.Fatalf("unexpected generated dispatcher:\n%s", generated)
	}
}

func TestParseAppMethodsValidatesBuildTagVariants(t *testing.T) {
	base := map[string]string{
		"a.go": "//go:build one\npackage main\ntype App struct{}\n//kubikles:dispatch exclude\nfunc (*App) Variant(value string) {}\n",
		"b.go": "//go:build !one\npackage main\n//kubikles:dispatch exclude\nfunc (*App) Variant(value string) {}\n",
	}
	methods, _ := fixtureMethods(t, base)
	if len(methods) != 1 || strings.Join(methods[0].Sources, ",") != "a.go,b.go" {
		t.Fatalf("variants = %#v", methods)
	}
	for name, replacement := range map[string]string{
		"signature": "//go:build !one\npackage main\nfunc (*App) Variant(value int) {}\n",
		"exclusion": "//go:build !one\npackage main\nfunc (*App) Variant(value string) {}\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			for filename, source := range base {
				if filename == "b.go" {
					source = replacement
				}
				writeFixture(t, dir, filename, source)
			}
			_, err := parseAppMethods(token.NewFileSet(), dir)
			if err == nil || !strings.Contains(err.Error(), "Variant") || !strings.Contains(err.Error(), "a.go") || !strings.Contains(err.Error(), "b.go") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestParseAppMethodsRejectsInvalidDirectiveAndImplicitVariadic(t *testing.T) {
	for name, test := range map[string]struct {
		source string
		want   string
	}{
		"unknown":  {"package main\ntype App struct{}\n//kubikles:dispatch nope\nfunc (*App) Method() {}\n", `Method in app.go: unknown dispatch directive "nope"`},
		"conflict": {"package main\ntype App struct{}\n//kubikles:dispatch exclude\n//kubikles:dispatch include\nfunc (*App) Method() {}\n", "Method in app.go: conflicting dispatch directives"},
		"variadic": {"package main\ntype App struct{}\nfunc (*App) Method(values ...string) {}\n", "variadic method"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFixture(t, dir, "app.go", test.source)
			if _, err := parseAppMethods(token.NewFileSet(), dir); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
	methods, _ := fixtureMethods(t, map[string]string{"app.go": "package main\ntype App struct{}\n//kubikles:dispatch exclude\nfunc (*App) Method(values ...string) {}\n"})
	if !methods[0].Excluded || strings.Contains(string(generateDispatcher(methods)), `case "Method"`) {
		t.Fatal("excluded variadic method must not generate a case")
	}
}

func TestParseAppMethodsOnlyRecognizesExactAttachedDirectives(t *testing.T) {

	methods, _ := fixtureMethods(t, map[string]string{"app.go": `package main
type App struct{}
// kubikles:dispatch exclude
func (*App) Spaced() {}
//kubikles:dispatcher documentation
func (*App) PrefixLike() {}
//kubikles:dispatch exclude
func (*App) Exact() {}
`})
	got := map[string]bool{}
	for _, method := range methods {
		got[method.Name] = method.Excluded
	}
	if got["Spaced"] || got["PrefixLike"] || !got["Exact"] {
		t.Fatalf("directive classification = %#v", got)
	}
}

func TestParseAppMethodsRejectsDispatchDirectiveNearMatches(t *testing.T) {
	for name, directive := range map[string]string{
		"double space":        "//kubikles:dispatch  exclude",
		"tab":                 "//kubikles:dispatch\texclude",
		"trailing whitespace": "//kubikles:dispatch exclude ",
		"missing token":       "//kubikles:dispatch",
		"extra token":         "//kubikles:dispatch exclude extra",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFixture(t, dir, "app.go", "package main\ntype App struct{}\n"+directive+"\nfunc (*App) Method() {}\n")
			_, err := parseAppMethods(token.NewFileSet(), dir)
			want := fmt.Sprintf("Method in app.go: invalid dispatch directive %q", strings.TrimPrefix(directive, "//"))
			if err == nil || err.Error() != want {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestGenerateDispatcherImportsOnlyEligibleQualifiedTypes(t *testing.T) {
	methods, _ := fixtureMethods(t, map[string]string{"app.go": `package main
import "kubikles/pkg/helm"
type App struct{}
//kubikles:dispatch exclude
func (*App) Excluded(value helm.Release) {}
func (*App) Included(value helm.Release) {}
`})
	generated := string(generateDispatcher(methods))
	if strings.Count(generated, `"kubikles/pkg/helm"`) != 1 || strings.Contains(generated, `case "Excluded"`) || !strings.Contains(generated, `case "Included"`) {
		t.Fatalf("unexpected generated dispatcher:\n%s", generated)
	}

	methods, _ = fixtureMethods(t, map[string]string{"app.go": `package main
import "kubikles/pkg/helm"
type App struct{}
//kubikles:dispatch exclude
func (*App) Excluded(value helm.Release) {}
`})
	generated = string(generateDispatcher(methods))
	if strings.Contains(generated, `"kubikles/pkg/helm"`) {
		t.Fatalf("excluded-only qualifier leaked import:\n%s", generated)
	}
}

func TestParseAppMethodsRejectsUnresolvedEligibleQualifier(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "app.go", "package main\ntype App struct{}\nfunc (*App) Timed(value time.Duration) {}\n")
	_, err := parseAppMethods(token.NewFileSet(), dir)
	if err == nil || !strings.Contains(err.Error(), "Timed") || !strings.Contains(err.Error(), "app.go") || !strings.Contains(err.Error(), "time") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseAppMethodsPreservesOrRejectsComplexTypesWithoutLoss(t *testing.T) {
	t.Run("mismatched variants", func(t *testing.T) {
		dir := t.TempDir()
		writeFixture(t, dir, "a.go", "//go:build one\npackage main\ntype App struct{}\nfunc (*App) Variant(value chan string) {}\n")
		writeFixture(t, dir, "b.go", "//go:build !one\npackage main\nfunc (*App) Variant(value func()) {}\n")
		_, err := parseAppMethods(token.NewFileSet(), dir)
		if err == nil || !strings.Contains(err.Error(), "Variant") || !strings.Contains(err.Error(), "a.go") || !strings.Contains(err.Error(), "b.go") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("consistent channel", func(t *testing.T) {
		methods, _ := fixtureMethods(t, map[string]string{"app.go": "package main\ntype App struct{}\nfunc (*App) Channel(value chan string) {}\n"})
		generated, err := format.Source(generateDispatcher(methods))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(generated), "unmarshalArg[chan string]") || strings.Contains(string(generated), "unmarshalArg[interface{}]") {
			t.Fatalf("complex type was lost:\n%s", generated)
		}
	})
}

func TestGenerateDispatcherInjectsAuthenticatedContext(t *testing.T) {
	methods, _ := fixtureMethods(t, map[string]string{"app.go": `package main
import "kubikles/pkg/agent"
type App struct{}
func (*App) SubscribeSecretWatcher(callContext agent.AuthenticatedCallContext, subscriptionID, namespace string) error { return nil }
`})
	generated := string(generateDispatcher(methods))
	for _, want := range []string{
		"CallMethod(callContext agent.AuthenticatedCallContext, methodName string, args []json.RawMessage)",
		"unmarshalArg[string](args, 0)", "unmarshalArg[string](args, 1)",
		"c.app.SubscribeSecretWatcher(callContext, p0, p1)",
	} {
		if !strings.Contains(generated, want) {
			t.Errorf("generated dispatcher missing %q", want)
		}
	}
	for _, source := range []string{
		"package main\nimport auth \"kubikles/pkg/agent\"\ntype App struct{}\nfunc (*App) Bad(value string, callContext auth.AuthenticatedCallContext) {}\n",
		"package main\nimport auth \"kubikles/pkg/agent\"\ntype App struct{}\nfunc (*App) Bad(first auth.AuthenticatedCallContext, second auth.AuthenticatedCallContext) {}\n",
	} {
		dir := t.TempDir()
		writeFixture(t, dir, "app.go", source)
		if _, err := parseAppMethods(token.NewFileSet(), dir); err == nil {
			t.Error("expected invalid context position error")
		}
	}
}

func TestGenerateDispatcherTrustedContextHasExactArgumentShapes(t *testing.T) {
	methods, _ := fixtureMethods(t, map[string]string{"app.go": `package main
import "kubikles/pkg/agent"
type App struct{}
func (*App) Two(callContext agent.AuthenticatedCallContext, first, second string) {}
func (*App) One(callContext agent.AuthenticatedCallContext, only string) {}
`})
	generated, err := format.Source(generateDispatcher(methods))
	if err != nil {
		t.Fatal(err)
	}
	text := string(generated)
	for _, want := range []string{
		"c.app.Two(callContext, p0, p1)",
		"c.app.One(callContext, p0)",
		"p0, err := unmarshalArg[string](args, 0)",
		"p1, err := unmarshalArg[string](args, 1)",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated dispatcher missing exact %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "unmarshalArg[agent.AuthenticatedCallContext]") || strings.Contains(text, "unmarshalArg[AuthenticatedCallContext]") {
		t.Fatalf("trusted context was decoded from caller-controlled args:\n%s", text)
	}
}

func TestGeneratedDispatcherClassifiesAuthenticatedContextByImportPath(t *testing.T) {
	methods, dir := fixtureMethods(t, map[string]string{"app.go": `package main
import (
    auth "kubikles/pkg/agent"
    wrongctx "dispatchfixture/wrong"
)
type App struct{}
func (*App) Trusted(ctx auth.AuthenticatedCallContext) string {
    return string(ctx.PrincipalID) + ":" + string(ctx.SessionID)
}
func (*App) Homonym(ctx wrongctx.AuthenticatedCallContext) string {
    return ctx.PrincipalID + ":" + ctx.SessionID
}
`})
	if err := os.MkdirAll(filepath.Join(dir, "wrong"), 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(dir, "wrong"), "context.go", `package wrong
type AuthenticatedCallContext struct {
    PrincipalID string
    SessionID string
}
`)
	generated, err := format.Source(generateDispatcher(methods))
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dir, "dispatch_gen.go", string(generated))
	writeFixture(t, dir, "dispatch_test.go", `package main
import (
    "encoding/json"
    "testing"
    "kubikles/pkg/agent"
)
func TestSemanticContextDispatch(t *testing.T) {
    caller := NewAppMethodCaller(&App{})
    trusted := agent.AuthenticatedCallContext{PrincipalID: "server", SessionID: "trusted"}
	forged := json.RawMessage([]byte("{\"PrincipalID\":\"forged\",\"SessionID\":\"caller\"}"))
    result, err := caller.CallMethod(trusted, "Trusted", []json.RawMessage{forged})
    if err != nil || result != "server:trusted" { t.Fatalf("trusted result, err = %#v, %v", result, err) }
    result, err = caller.CallMethod(trusted, "Homonym", []json.RawMessage{forged})
    if err != nil || result != "forged:caller" { t.Fatalf("homonym result, err = %#v, %v", result, err) }
}
`)
	root := repositoryRoot(t)
	writeFixture(t, dir, "go.mod", "module dispatchfixture\n\ngo 1.24\n\nrequire kubikles v0.0.0\n\nreplace kubikles => "+root+"\n")
	cmd := exec.Command("go", "test", "-mod=mod", ".")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated fixture did not compile or run: %v\n%s", err, output)
	}
}

func TestParseAppMethodsTreatsTrustedContextAliasesAsEquivalentVariants(t *testing.T) {
	methods, _ := fixtureMethods(t, map[string]string{
		"a.go": "//go:build one\npackage main\nimport auth \"kubikles/pkg/agent\"\ntype App struct{}\nfunc (*App) Method(ctx auth.AuthenticatedCallContext) {}\n",
		"b.go": "//go:build !one\npackage main\nimport agent \"kubikles/pkg/agent\"\nfunc (*App) Method(ctx agent.AuthenticatedCallContext) {}\n",
	})
	if len(methods) != 1 || len(methods[0].Params) != 1 || !methods[0].Params[0].TrustedContext {
		t.Fatalf("methods = %#v", methods)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func repositoryMethods(t *testing.T) ([]methodInfo, string) {
	t.Helper()
	root := repositoryRoot(t)
	methods, err := parseAppMethods(token.NewFileSet(), root)
	if err != nil {
		t.Fatal(err)
	}
	return methods, root
}

func TestRepositoryDispatchClassificationExhaustive(t *testing.T) {
	methods, _ := repositoryMethods(t)
	generated := string(generateDispatcher(methods))
	for _, method := range methods {
		caseText := `case "` + method.Name + `"`
		if method.Excluded == strings.Contains(generated, caseText) {
			t.Errorf("method %s classification does not match generated cases", method.Name)
		}
	}
	for _, name := range []string{"IsHelmAvailable", "IsDebugClusterEnabled"} {
		count := 0
		for _, method := range methods {
			if method.Name == name {
				count++
			}
		}
		if count != 1 {
			t.Errorf("%s group count = %d, want 1", name, count)
		}
	}
}

func TestGeneratedDispatcherIsCurrent(t *testing.T) {
	methods, root := repositoryMethods(t)
	want, err := format.Source(generateDispatcher(methods))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "dispatch_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatal("dispatch_gen.go is stale; run make generate")
	}
}

func TestAcceleratorPolicyIsSeparatePositiveLookup(t *testing.T) {
	methods, _ := repositoryMethods(t)
	eligible := make(map[string]bool)
	for _, method := range methods {
		eligible[method.Name] = !method.Excluded
	}
	for _, test := range []struct {
		name   string
		policy bool
	}{
		{"ListSecretsMetadata", true},
		{"ListPods", false},
		{"UpdateSecretData", false},
	} {
		if !eligible[test.name] {
			t.Errorf("%s is not ordinary-server eligible", test.name)
		}
		_, policy := agent.LookupMethodPolicy(test.name)
		if policy != test.policy {
			t.Errorf("LookupMethodPolicy(%q) = %v, want %v", test.name, policy, test.policy)
		}
	}
	if strings.Contains(string(generateDispatcher(methods)), "LookupMethodPolicy") {
		t.Error("generated dispatch must not implement Accelerator policy")
	}
}
