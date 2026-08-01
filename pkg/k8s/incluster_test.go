package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func testRESTConfig(t *testing.T) *rest.Config {
	t.Helper()
	return &rest.Config{Host: "https://fixed.example"}
}

func TestNewClientForRESTConfigRejectsNil(t *testing.T) {
	_, err := NewClientForRESTConfig(nil)
	if !errors.Is(err, ErrNilRESTConfig) {
		t.Fatalf("NewClientForRESTConfig(nil) error = %v, want ErrNilRESTConfig", err)
	}
}

func TestNewClientForRESTConfigCopiesConfig(t *testing.T) {
	source := &rest.Config{Host: "https://original.example", BearerToken: "token"}
	client, err := NewClientForRESTConfig(source)
	if err != nil {
		t.Fatalf("NewClientForRESTConfig() error = %v", err)
	}
	source.Host = "https://changed.example"
	first, err := client.GetRestConfigForContext(InClusterContextName)
	if err != nil {
		t.Fatalf("GetRestConfigForContext() error = %v", err)
	}
	second, err := client.GetRestConfigForContext("")
	if err != nil {
		t.Fatalf("GetRestConfigForContext(\"\") error = %v", err)
	}
	if first == second || first.Host != "https://original.example" {
		t.Fatalf("fixed config = %#v, want independent original copies", first)
	}
	first.Host = "https://mutated.example"
	if second.Host != "https://original.example" {
		t.Fatalf("config copy mutation leaked: %q", second.Host)
	}
}

func TestNewClientForRESTConfigDeepCopiesNestedConfig(t *testing.T) {
	source := &rest.Config{
		Host: "https://original.example",
		TLSClientConfig: rest.TLSClientConfig{
			NextProtos: []string{"h2", "http/1.1"},
		},
		Impersonate: rest.ImpersonationConfig{
			Groups: []string{"original-group"},
			Extra:  map[string][]string{"scope": {"original-extra"}},
		},
	}

	client, err := NewClientForRESTConfig(source)
	if err != nil {
		t.Fatalf("NewClientForRESTConfig() error = %v", err)
	}

	source.TLSClientConfig.NextProtos[0] = "http/1.1"
	source.Impersonate.Groups[0] = "changed-group"
	source.Impersonate.Extra["scope"][0] = "changed-extra"

	first, err := client.GetRestConfigForContext(InClusterContextName)
	if err != nil {
		t.Fatalf("GetRestConfigForContext() error = %v", err)
	}
	if got, want := first.TLSClientConfig.NextProtos, []string{"h2", "http/1.1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("NextProtos = %q, want %q", got, want)
	}
	if got, want := first.Impersonate.Groups, []string{"original-group"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Impersonate.Groups = %q, want %q", got, want)
	}
	if got, want := first.Impersonate.Extra, map[string][]string{"scope": {"original-extra"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Impersonate.Extra = %q, want %q", got, want)
	}

	first.TLSClientConfig.NextProtos[0] = "mutated"
	first.Impersonate.Groups[0] = "mutated-group"
	first.Impersonate.Extra["scope"][0] = "mutated-extra"

	second, err := client.GetRestConfigForContext(InClusterContextName)
	if err != nil {
		t.Fatalf("GetRestConfigForContext() error = %v", err)
	}
	if got, want := second.Impersonate.Groups, []string{"original-group"}; !reflect.DeepEqual(got, want) {
		t.Errorf("later Impersonate.Groups = %q, want %q", got, want)
	}
	if got, want := second.TLSClientConfig.NextProtos, []string{"h2", "http/1.1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("later NextProtos = %q, want %q", got, want)
	}
	if got, want := second.Impersonate.Extra, map[string][]string{"scope": {"original-extra"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("later Impersonate.Extra = %q, want %q", got, want)
	}
}

func TestDeepCopyRESTConfigCopiesProviderState(t *testing.T) {
	source := &rest.Config{
		ContentConfig: rest.ContentConfig{GroupVersion: &schema.GroupVersion{Group: "original", Version: "v1"}},
		TLSClientConfig: rest.TLSClientConfig{
			CAData:     []byte("original-ca"),
			CertData:   []byte("original-cert"),
			KeyData:    []byte("original-key"),
			NextProtos: []string{"h2", "http/1.1"},
		},
		AuthProvider: &clientcmdapi.AuthProviderConfig{Config: map[string]string{"key": "original"}},
		ExecProvider: &clientcmdapi.ExecConfig{
			Args: []string{"original-arg"},
			Env:  []clientcmdapi.ExecEnvVar{{Name: "KEY", Value: "original-value"}},
		},
	}
	copy := deepCopyRESTConfig(source)

	source.AuthProvider.Config["key"] = "source-mutation"
	source.GroupVersion.Group = "source-mutation"
	source.TLSClientConfig.CAData[0] = 'X'
	source.TLSClientConfig.CertData[0] = 'X'
	source.TLSClientConfig.KeyData[0] = 'X'
	source.TLSClientConfig.NextProtos[0] = "source-mutation"
	source.ExecProvider.Args[0] = "source-mutation"
	source.ExecProvider.Env[0].Value = "source-mutation"
	if got, want := copy.AuthProvider.Config["key"], "original"; got != want {
		t.Errorf("AuthProvider.Config[key] = %q, want %q", got, want)
	}
	if got, want := copy.GroupVersion.Group, "original"; got != want {
		t.Errorf("GroupVersion.Group = %q, want %q", got, want)
	}
	if got, want := copy.TLSClientConfig.CAData, []byte("original-ca"); !reflect.DeepEqual(got, want) {
		t.Errorf("CAData = %q, want %q", got, want)
	}
	if got, want := copy.TLSClientConfig.CertData, []byte("original-cert"); !reflect.DeepEqual(got, want) {
		t.Errorf("CertData = %q, want %q", got, want)
	}
	if got, want := copy.TLSClientConfig.KeyData, []byte("original-key"); !reflect.DeepEqual(got, want) {
		t.Errorf("KeyData = %q, want %q", got, want)
	}
	if got, want := copy.TLSClientConfig.NextProtos, []string{"h2", "http/1.1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("NextProtos = %q, want %q", got, want)
	}
	if got, want := copy.ExecProvider.Args, []string{"original-arg"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ExecProvider.Args = %q, want %q", got, want)
	}
	if got, want := copy.ExecProvider.Env, []clientcmdapi.ExecEnvVar{{Name: "KEY", Value: "original-value"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("ExecProvider.Env = %q, want %q", got, want)
	}

	copy.AuthProvider.Config["key"] = "copy-mutation"
	copy.GroupVersion.Group = "copy-mutation"
	copy.ExecProvider.Args[0] = "copy-mutation"
	copy.ExecProvider.Env[0].Value = "copy-mutation"
	if got, want := source.AuthProvider.Config["key"], "source-mutation"; got != want {
		t.Errorf("source AuthProvider.Config[key] = %q, want %q", got, want)
	}
	if got, want := source.GroupVersion.Group, "source-mutation"; got != want {
		t.Errorf("source GroupVersion.Group = %q, want %q", got, want)
	}
	if got, want := source.ExecProvider.Args, []string{"source-mutation"}; !reflect.DeepEqual(got, want) {
		t.Errorf("source ExecProvider.Args = %q, want %q", got, want)
	}
	if got, want := source.ExecProvider.Env, []clientcmdapi.ExecEnvVar{{Name: "KEY", Value: "source-mutation"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("source ExecProvider.Env = %q, want %q", got, want)
	}
}

func TestDeepCopyRESTConfigCopiesExecProviderRuntimeConfig(t *testing.T) {
	tests := []struct {
		name      string
		newObject func() runtime.Object
		mutate    func(runtime.Object, string)
		value     func(runtime.Object) string
	}{
		{
			name: "runtime unknown",
			newObject: func() runtime.Object {
				return &runtime.Unknown{Raw: []byte("original")}
			},
			mutate: func(object runtime.Object, value string) {
				object.(*runtime.Unknown).Raw = []byte(value)
			},
			value: func(object runtime.Object) string {
				return string(object.(*runtime.Unknown).Raw)
			},
		},
		{
			name: "unstructured",
			newObject: func() runtime.Object {
				return &unstructured.Unstructured{Object: map[string]interface{}{"value": "original"}}
			},
			mutate: func(object runtime.Object, value string) {
				object.(*unstructured.Unstructured).Object["value"] = value
			},
			value: func(object runtime.Object) string {
				return object.(*unstructured.Unstructured).Object["value"].(string)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := &rest.Config{ExecProvider: &clientcmdapi.ExecConfig{
				APIVersion: "client.authentication.k8s.io/v1",
				Command:    "example",
				Config:     test.newObject(),
			}}
			sourceExecProvider := source.ExecProvider
			sourceRuntimeConfig := source.ExecProvider.Config
			stored := deepCopyRESTConfig(source)
			if source.ExecProvider != sourceExecProvider {
				t.Fatal("deepCopyRESTConfig changed the source ExecProvider pointer")
			}
			if source.ExecProvider.Config != sourceRuntimeConfig {
				t.Fatal("deepCopyRESTConfig changed the source ExecProvider.Config pointer")
			}
			if got, want := test.value(source.ExecProvider.Config), "original"; got != want {
				t.Fatalf("source ExecProvider.Config after copy = %q, want %q", got, want)
			}
			if got, want := test.value(stored.ExecProvider.Config), "original"; got != want {
				t.Fatalf("stored ExecProvider.Config = %q, want %q", got, want)
			}
			if stored.ExecProvider.Config == source.ExecProvider.Config {
				t.Fatal("stored ExecProvider.Config shares the source runtime object")
			}

			test.mutate(source.ExecProvider.Config, "source-mutation")
			if got, want := test.value(stored.ExecProvider.Config), "original"; got != want {
				t.Errorf("stored ExecProvider.Config after source mutation = %q, want %q", got, want)
			}
			test.mutate(stored.ExecProvider.Config, "stored-mutation")
			if got, want := test.value(source.ExecProvider.Config), "source-mutation"; got != want {
				t.Errorf("source ExecProvider.Config after stored mutation = %q, want %q", got, want)
			}

			sourceExecProvider = source.ExecProvider
			sourceRuntimeConfig = source.ExecProvider.Config
			client, err := NewClientForRESTConfig(source)
			if err != nil {
				t.Fatalf("NewClientForRESTConfig() error = %v", err)
			}
			if source.ExecProvider != sourceExecProvider {
				t.Fatal("NewClientForRESTConfig changed the source ExecProvider pointer")
			}
			if source.ExecProvider.Config != sourceRuntimeConfig {
				t.Fatal("NewClientForRESTConfig changed the source ExecProvider.Config pointer")
			}
			if got, want := test.value(source.ExecProvider.Config), "source-mutation"; got != want {
				t.Fatalf("source ExecProvider.Config after construction = %q, want %q", got, want)
			}
			first, err := client.GetRestConfigForContext(InClusterContextName)
			if err != nil {
				t.Fatalf("first GetRestConfigForContext() error = %v", err)
			}
			second, err := client.GetRestConfigForContext(InClusterContextName)
			if err != nil {
				t.Fatalf("second GetRestConfigForContext() error = %v", err)
			}
			if first.ExecProvider.Config == second.ExecProvider.Config {
				t.Fatal("successive getters share ExecProvider.Config")
			}
			if got, want := test.value(first.ExecProvider.Config), "source-mutation"; got != want {
				t.Errorf("first ExecProvider.Config = %q, want %q", got, want)
			}
			test.mutate(first.ExecProvider.Config, "getter-mutation")
			if got, want := test.value(second.ExecProvider.Config), "source-mutation"; got != want {
				t.Errorf("second ExecProvider.Config after first mutation = %q, want %q", got, want)
			}
			if got, want := test.value(source.ExecProvider.Config), "source-mutation"; got != want {
				t.Errorf("source ExecProvider.Config after getter mutation = %q, want %q", got, want)
			}
		})
	}
}

func TestClientRESTConfigGettersDoNotAliasExecProviderRuntimeConfig(t *testing.T) {
	tests := []struct {
		name      string
		newObject func() runtime.Object
		mutate    func(runtime.Object, int)
		value     func(runtime.Object) int
	}{
		{
			name:      "runtime unknown",
			newObject: func() runtime.Object { return &runtime.Unknown{Raw: []byte{0, 0}} },
			mutate: func(object runtime.Object, iteration int) {
				object.(*runtime.Unknown).Raw = []byte{byte(iteration >> 8), byte(iteration)}
			},
			value: func(object runtime.Object) int {
				raw := object.(*runtime.Unknown).Raw
				return int(raw[0])<<8 | int(raw[1])
			},
		},
		{
			name: "unstructured",
			newObject: func() runtime.Object {
				return &unstructured.Unstructured{Object: map[string]interface{}{"value": float64(0)}}
			},
			mutate: func(object runtime.Object, iteration int) {
				object.(*unstructured.Unstructured).Object["value"] = float64(iteration)
			},
			value: func(object runtime.Object) int {
				return int(object.(*unstructured.Unstructured).Object["value"].(float64))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := &rest.Config{Host: "https://fixed.example", ExecProvider: &clientcmdapi.ExecConfig{
				APIVersion: "client.authentication.k8s.io/v1",
				Command:    "example",
				Config:     test.newObject(),
			}}
			client, err := NewClientForRESTConfig(source)
			if err != nil {
				t.Fatalf("NewClientForRESTConfig() error = %v", err)
			}

			storedExecProvider := client.baseConfig.ExecProvider
			storedRuntimeConfig := client.baseConfig.ExecProvider.Config
			start := make(chan struct{})
			var group sync.WaitGroup
			const getterCount = 8
			group.Add(getterCount)
			results := make(chan *rest.Config, getterCount)
			for getter := 0; getter < getterCount; getter++ {
				getter := getter
				go func() {
					defer group.Done()
					<-start
					var last *rest.Config
					for iteration := 0; iteration < 1_000; iteration++ {
						config, err := client.GetRestConfigForContext(InClusterContextName)
						if err != nil {
							t.Errorf("GetRestConfigForContext() error = %v", err)
							return
						}
						test.mutate(config.ExecProvider.Config, getter+1)
						last = config
					}
					results <- last
				}()
			}
			close(start)
			group.Wait()
			close(results)

			if client.baseConfig.ExecProvider != storedExecProvider {
				t.Fatal("concurrent getters changed the stored ExecProvider pointer")
			}
			if client.baseConfig.ExecProvider.Config != storedRuntimeConfig {
				t.Fatal("concurrent getters changed the stored ExecProvider.Config pointer")
			}
			if got, want := test.value(client.baseConfig.ExecProvider.Config), 0; got != want {
				t.Fatalf("stored ExecProvider.Config value = %d, want %d", got, want)
			}
			seen := make(map[runtime.Object]struct{}, getterCount)
			for config := range results {
				if config.ExecProvider == storedExecProvider {
					t.Error("getter returned the stored ExecProvider pointer")
				}
				if config.ExecProvider.Config == storedRuntimeConfig {
					t.Error("getter returned the stored ExecProvider.Config pointer")
				}
				if _, exists := seen[config.ExecProvider.Config]; exists {
					t.Error("getter results share an ExecProvider.Config pointer")
				}
				seen[config.ExecProvider.Config] = struct{}{}
			}
		})
	}
}

func TestNewClientForRESTConfigIsolatesActiveImpersonationTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if got, want := r.Header.Values("Impersonate-Group"), []string{"original-group"}; !reflect.DeepEqual(got, want) {
			t.Errorf("Impersonate-Group = %q, want %q", got, want)
		}
		if got, want := r.Header.Values("Impersonate-Extra-Scope"), []string{"original-extra"}; !reflect.DeepEqual(got, want) {
			t.Errorf("Impersonate-Extra-Scope = %q, want %q", got, want)
		}
		_ = json.NewEncoder(w).Encode(corev1.PodList{})
	}))
	defer server.Close()

	source := &rest.Config{
		Host: server.URL,
		Impersonate: rest.ImpersonationConfig{
			UserName: "original-user",
			Groups:   []string{"original-group"},
			Extra:    map[string][]string{"scope": {"original-extra"}},
		},
	}
	client, err := NewClientForRESTConfig(source)
	if err != nil {
		t.Fatalf("NewClientForRESTConfig() error = %v", err)
	}
	source.Impersonate.Groups[0] = "source-mutation"
	source.Impersonate.Extra["scope"][0] = "source-mutation"

	returned, err := client.GetRestConfigForContext(InClusterContextName)
	if err != nil {
		t.Fatalf("GetRestConfigForContext() error = %v", err)
	}
	returned.Impersonate.Groups[0] = "getter-mutation"
	returned.Impersonate.Extra["scope"][0] = "getter-mutation"

	if _, err := client.clientset.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{}); err != nil {
		t.Fatalf("Pods().List() error = %v", err)
	}
}

func TestNewClientForRESTConfigUsesSecretReadPaths(t *testing.T) {
	secret := corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "example", Namespace: "default"}, Data: map[string][]byte{"key": []byte("value")}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/namespaces/default/secrets" && r.URL.Query().Get("watch") == "true":
			watchSecret := secret.DeepCopy()
			watchSecret.APIVersion, watchSecret.Kind = "v1", "Secret"
			_ = json.NewEncoder(w).Encode(map[string]any{"type": "ADDED", "object": watchSecret})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/namespaces/default/secrets":
			if r.Header.Get("Accept") != "application/json;as=Table;g=meta.k8s.io;v=v1" {
				t.Errorf("metadata list Accept = %q", r.Header.Get("Accept"))
			}
			raw, _ := json.Marshal(map[string]any{"metadata": map[string]any{"name": secret.Name, "namespace": secret.Namespace}})
			_ = json.NewEncoder(w).Encode(metav1.Table{Rows: []metav1.TableRow{{Cells: []interface{}{secret.Name, "Opaque", 1}, Object: runtime.RawExtension{Raw: raw}}}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/namespaces/default/secrets/example":
			_ = json.NewEncoder(w).Encode(secret)
		default:
			t.Errorf("unexpected Kubernetes request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewClientForRESTConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatalf("NewClientForRESTConfig() error = %v", err)
	}
	items, err := client.ListSecretsMetadataWithContext(context.Background(), "default")
	if err != nil || len(items) != 1 || items[0].Metadata.Name != secret.Name {
		t.Fatalf("ListSecretsMetadataWithContext() = %#v, %v", items, err)
	}
	data, err := client.GetSecretData("default", secret.Name)
	if err != nil || len(data) != 1 || data[0].Value != "value" {
		t.Fatalf("GetSecretData() = %#v, %v", data, err)
	}
	stream, err := client.WatchResource(context.Background(), "secrets", "default", "")
	if err != nil {
		t.Fatalf("WatchResource() error = %v", err)
	}
	defer stream.Stop()
	select {
	case event := <-stream.ResultChan():
		got, ok := event.Object.(*corev1.Secret)
		if event.Type != watch.Added || !ok || got.Name != secret.Name {
			t.Fatalf("watch event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("watch event not received")
	}
}
