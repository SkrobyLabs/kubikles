package query

import (
	"reflect"
	"strings"
	"testing"
)

type cond struct {
	Type    string
	Field   string
	Value   string
	IsRegex bool
}

func flat(q Query) [][]cond {
	var out [][]cond
	for _, g := range q.Groups {
		var row []cond
		for _, c := range g {
			row = append(row, cond{c.Type, c.Field, c.Value, c.IsRegex})
		}
		out = append(out, row)
	}
	return out
}

func TestParseQueryVectors(t *testing.T) {
	plain := func(v string) cond { return cond{"plain", "", v, false} }
	field := func(f, v string) cond { return cond{"field", f, v, false} }
	cases := []struct {
		in   string
		want [][]cond
	}{
		// Ported from queryParser.test.js.
		{"", nil},
		{"   ", nil},
		{"nginx", [][]cond{{plain("nginx")}}},
		{"nginx redis", [][]cond{{plain("nginx"), plain("redis")}}},
		{"  nginx   redis  ", [][]cond{{plain("nginx"), plain("redis")}}},
		{"name:nginx", [][]cond{{field("name", "nginx")}}},
		{`name:"my-pod"`, [][]cond{{field("name", "my-pod")}}},
		{"name:'my-pod'", [][]cond{{field("name", "my-pod")}}},
		{`name:""`, [][]cond{{field("name", "")}}},
		{"Name:nginx NodeName:node1", [][]cond{{field("name", "nginx"), field("nodename", "node1")}}},
		{"name:nginx status:Running", [][]cond{{field("name", "nginx"), field("status", "Running")}}},
		{"api name:nginx status:Running", [][]cond{{plain("api"), field("name", "nginx"), field("status", "Running")}}},
		{`label:"app = nginx"`, [][]cond{{field("label", "app = nginx")}}},
		{"/[invalid/", [][]cond{{plain("/[invalid/")}}},
		{"name:/[invalid/", [][]cond{{field("name", "/[invalid/")}}},
		{"/^nginx-/", [][]cond{{{"plain", "", "/^nginx-/", true}}}},
		{"name:/nginx/gi", [][]cond{{{"field", "name", "/nginx/gi", true}}}},
		{"name:/a/ OR name:/b/", [][]cond{{{"field", "name", "/a/", true}}, {{"field", "name", "/b/", true}}}},
		// Beyond the ported vectors: separators, fallbacks and quirks of the regexes.
		{"a or b", [][]cond{{plain("a")}, {plain("b")}}},
		{"a  OR  b", [][]cond{{plain("a")}, {plain("b")}}},
		{"a OR", [][]cond{{plain("a"), plain("OR")}}},
		{" OR a", [][]cond{{plain("OR"), plain("a")}}},
		{"a OR ", [][]cond{{plain("a"), plain("OR")}}},
		{"a ORb", [][]cond{{plain("a"), plain("ORb")}}},
		{"name:/a/gg", [][]cond{{field("name", "/a/gg")}}},
		{"name:/a/x", [][]cond{{field("name", "/a/x")}}},
		{"name:a,b", [][]cond{{field("name", "a"), plain(",b")}}},
		{"a-b:c", [][]cond{{plain("a-"), field("b", "c")}}},
		{":abc", [][]cond{{plain(":abc")}}},
		{`name:"open`, [][]cond{{field("name", `"open`)}}},
		// The word regex backtracks so a field-looking tail can start a new token.
		{"name:", [][]cond{{plain("nam"), plain("e:")}}},
		{"http://x", [][]cond{{field("http", "//x")}}},
		{"a\u00a0b", [][]cond{{plain("a"), plain("b")}}},
	}
	for _, c := range cases {
		got := flat(ParseQuery(c.in))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParseQuery(%q) = %#v, want %#v", c.in, got, c.want)
		}
	}
}

type item struct {
	name, ns, status, node string
}

var pods = []item{
	{"nginx-abc123", "default", "Running", "worker-1"},
	{"nginx-def456", "default", "Pending", "worker-2"},
	{"redis-xyz789", "kube-system", "Running", "worker-1"},
	{"api-server", "production", "Running", "worker-3"},
}

var podFields = []Field[item]{
	{Name: "name", Aliases: []string{"n"}, Extract: func(i item) string { return i.name }},
	{Name: "namespace", Aliases: []string{"ns"}, Extract: func(i item) string { return i.ns }},
	{Name: "status", Aliases: []string{"phase", "s"}, Extract: func(i item) string { return i.status }},
	{Name: "nodename", Aliases: []string{"node", "n"}, Extract: func(i item) string { return i.node }},
}

func match(q string) []string {
	f := CompileString(q, podFields, func(i item) string { return i.name })
	var out []string
	for _, p := range pods {
		if f(p) {
			out = append(out, p.name)
		}
	}
	return out
}

func TestEngineVectors(t *testing.T) {
	nginx := []string{"nginx-abc123", "nginx-def456"}
	cases := []struct {
		q    string
		want []string
	}{
		// Ported from filterEngine.test.js and filterEngine.aliases.test.ts.
		{"", []string{"nginx-abc123", "nginx-def456", "redis-xyz789", "api-server"}},
		{"   ", []string{"nginx-abc123", "nginx-def456", "redis-xyz789", "api-server"}},
		{"nginx", nginx},
		{"NGINX", nginx},
		{"api", []string{"api-server"}},
		{"nonexistent", nil},
		{"namespace:kube-system", []string{"redis-xyz789"}},
		{"name:redis", []string{"redis-xyz789"}},
		{"nodename:worker-1", []string{"nginx-abc123", "redis-xyz789"}},
		{"status:Running", []string{"nginx-abc123", "redis-xyz789", "api-server"}},
		{"ns:default", nginx},
		{"NS:DEFAULT", nginx},
		{"PHASE:pending", []string{"nginx-def456"}},
		{"unknownfield:value", nil},
		{"name:nginx nosuchfield:nginx", nil},
		{"nosuchfield:web OR name:api", []string{"api-server"}},
		{"/^nginx/", nginx},
		{"/NGINX/i", nginx},
		{"/NGINX/", nil},
		{"/[invalid/", nil},
		{"/nginx/g", nginx},
		{"name:/^(nginx|redis)-/ ns:default", nginx},
		{"name:/^nginx-abc/ OR name:/^api/", []string{"nginx-abc123", "api-server"}},
		{"ns:default s:/^pend/i", []string{"nginx-def456"}},
		// "n" is both an alias of name and of nodename: the first declared field wins.
		{"n:redis", []string{"redis-xyz789"}},
		{"n:worker-3", nil},
		// Regexes RE2 cannot compile fall back to the backtracking engine.
		{"name:/^(?!nginx).*-s/", []string{"api-server"}},
		{"name:/(a)b?\\1/", nil},
		{"name:/(?<=nginx-)abc/", []string{"nginx-abc123"}},
		{"name:/^NGINX/y", nil},
		{"name:/^nginx/y", nginx},
		{"name:/abc/y", nil},
	}
	for _, c := range cases {
		got := match(c.q)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %v want %v", c.q, got, c.want)
		}
	}
}

func TestLookupPrefersDirectNames(t *testing.T) {
	fields := []Field[item]{
		{Name: "a", Aliases: []string{"x"}},
		{Name: "x"},
		{Name: "b", Aliases: []string{"x", "y"}},
	}
	if f := Lookup(fields, "X"); f == nil || f.Name != "x" {
		t.Fatalf("direct name must win, got %+v", f)
	}
	if f := Lookup(fields, "y"); f == nil || f.Name != "b" {
		t.Fatalf("alias resolves to its owner, got %+v", f)
	}
	if Lookup(fields, "nope") != nil {
		t.Fatal("unknown field must resolve to nil")
	}
}

func TestCompileJSEngines(t *testing.T) {
	cases := []struct {
		pattern, flags, engine string
		ok                     bool
	}{
		{"^abc", "", "re2", true},
		{"a(?=b)", "", "backtracking", true},
		{"(", "", "", false},
		{"a", "gg", "", false},
		{"a", "q", "", false},
		{"a.b", "s", "re2", true},
		{"(?<=a)b", "s", "backtracking", true},
	}
	for _, c := range cases {
		re, err := CompileJS(c.pattern, c.flags)
		if (err == nil) != c.ok {
			t.Errorf("CompileJS(%q,%q) error=%v, want ok=%v", c.pattern, c.flags, err, c.ok)
			continue
		}
		if c.ok && re.Engine() != c.engine {
			t.Errorf("CompileJS(%q,%q) engine %s, want %s", c.pattern, c.flags, re.Engine(), c.engine)
		}
	}
	re, _ := CompileJS("a.b", "s")
	if !re.Test("a\nb") {
		t.Error("dotAll flag must let . match a newline")
	}
	re, _ = CompileJS("(?<=a).b", "s")
	if !re.Test("a\nb") {
		t.Error("dotAll must survive the backtracking fallback")
	}
	re, _ = CompileJS("^b$", "m")
	if !re.Test("a\nb\nc") {
		t.Error("multiline flag")
	}
}

func TestBacktrackingTimeoutCountsAsNoMatch(t *testing.T) {
	re, err := CompileJS("^(?=(a+)+$)(a+)+$", "")
	if err != nil {
		t.Fatal(err)
	}
	if re.Engine() != "backtracking" {
		t.Fatalf("engine %s", re.Engine())
	}
	if re.Test(strings.Repeat("a", 40) + "!") {
		t.Fatal("must not match")
	}
}

func TestCompileJSFlaggedPatternsUseRE2(t *testing.T) {
	for _, c := range []struct{ pattern, flags string }{
		{"a.b", "i"},
		{`a\sb`, "i"},
		{`a\sb`, "m"},
	} {
		re, err := CompileJS(c.pattern, c.flags)
		if err != nil {
			t.Fatal(err)
		}
		if re.Engine() != "re2" {
			t.Errorf("CompileJS(%q,%q) engine %s, want re2", c.pattern, c.flags, re.Engine())
		}
	}
	re, _ := CompileJS(`A\sB`, "i")
	if !re.Test("a b") {
		t.Error("case-insensitive match failed")
	}
}
