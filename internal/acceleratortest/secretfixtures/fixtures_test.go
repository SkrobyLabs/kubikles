package secretfixtures

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
)

func TestAccelerator00SecretFixturesExact(t *testing.T) {
	secrets := Secrets()
	if len(secrets) != 7 {
		t.Fatal("fixture count")
	}
	counts := map[string]int{}
	for _, s := range secrets {
		counts[s.Namespace]++
	}
	if counts[NamespaceA] != 5 || counts[NamespaceB] != 2 {
		t.Fatal("namespace count")
	}
	want := []struct {
		name, uid, namespace string
		typ                  corev1.SecretType
		keys, bytes          int
	}{
		{"empty", "uid-empty", NamespaceA, corev1.SecretTypeOpaque, 0, 0}, {"text", "uid-text", NamespaceA, corev1.SecretTypeOpaque, 2, 288},
		{"binary", "uid-binary", NamespaceA, corev1.SecretTypeOpaque, 1, 1024}, {"helm", "uid-helm", NamespaceA, corev1.SecretType(HelmType), 1, 4096},
		{"churn", "uid-churn", NamespaceA, corev1.SecretTypeOpaque, 1, 16}, {"yaml", "uid-yaml", NamespaceB, corev1.SecretTypeOpaque, 2, 192},
		{"large", "uid-large", NamespaceB, corev1.SecretTypeOpaque, 1, 524288},
	}
	for i, s := range secrets {
		bytes := 0
		for _, value := range s.Data {
			bytes += len(value)
		}
		if s.Name != want[i].name || string(s.UID) != want[i].uid || s.Namespace != want[i].namespace || s.Type != want[i].typ || len(s.Data) != want[i].keys || bytes != want[i].bytes || !s.CreationTimestamp.Equal(&timestamp) {
			t.Fatal("fixture identity")
		}
	}
	if !utf8.Valid(secrets[1].Data["a"]) || utf8.Valid(secrets[2].Data["bin"]) || secrets[3].Type != HelmType || len(secrets[6].Data["blob"]) != 524288 {
		t.Fatal("fixture shape")
	}
	copy := Secrets()
	copy[1].Data["a"][0] = 'x'
	if bytes.Equal(copy[1].Data["a"], secrets[1].Data["a"]) {
		t.Fatal("fixtures must be deep copies")
	}
	events := ChurnEvents()
	if len(events) != 34 || events[0].UID != events[len(events)-1].UID || events[0].ResourceVersion != "" || events[1].ResourceVersion != "1" || events[32].ResourceVersion != "32" {
		t.Fatal("churn sequence")
	}
}

func TestAccelerator00SecretFixturesAreTestOnly(t *testing.T) {
	err := filepath.Walk("../../../", func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, readErr := os.ReadFile(path)
		if readErr == nil && bytes.Contains(body, []byte("kubikles/internal/acceleratortest/secretfixtures")) {
			t.Fatal("production fixture import")
		}
		return nil
	})
	if err != nil {
		t.Fatal("fixture import scan")
	}
}
