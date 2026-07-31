package k8s

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"kubikles/internal/acceleratortest/secretfixtures"
	yamlutil "sigs.k8s.io/yaml"
)

type accelerator00VisibleSecret struct {
	Name, Namespace, UID, CreationTimestamp, Type string
	DataKeys                                      int
}

func TestDirectSecretBaselineListAndNamespaces(t *testing.T) {
	secrets := secretfixtures.Secrets()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var selected []*corev1.Secret
		for _, secret := range secrets {
			if r.URL.Path == "/api/v1/secrets" || r.URL.Path == "/api/v1/namespaces/"+secret.Namespace+"/secrets" {
				selected = append(selected, secret)
			}
		}
		table := metav1.Table{ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name"}, {Name: "Type"}, {Name: "Data"}}}
		for _, secret := range selected {
			raw, _ := json.Marshal(map[string]any{"metadata": map[string]any{"name": secret.Name, "namespace": secret.Namespace, "uid": secret.UID, "creationTimestamp": secret.CreationTimestamp}})
			table.Rows = append(table.Rows, metav1.TableRow{Cells: []interface{}{secret.Name, string(secret.Type), len(secret.Data)}, Object: runtime.RawExtension{Raw: raw}})
		}
		_ = json.NewEncoder(w).Encode(table)
	}))
	defer server.Close()
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal("client")
	}
	client := &Client{clientset: cs}
	for namespace, want := range map[string]int{secretfixtures.NamespaceA: 5, secretfixtures.NamespaceB: 2, "": 7} {
		got, err := client.ListSecretsMetadataWithContext(context.Background(), namespace)
		if err != nil || len(got) != want {
			t.Fatal("list observation")
		}
		actual := map[string]accelerator00VisibleSecret{}
		var total int
		for _, item := range got {
			observation, err := json.Marshal(accelerator00VisibleSecret{item.Metadata.Name, item.Metadata.Namespace, item.Metadata.UID, item.Metadata.CreationTimestamp.UTC().Format(time.RFC3339), item.Type, item.DataKeys})
			if err != nil || len(observation) > 512 {
				t.Fatal("visible observation bound")
			}
			visible := accelerator00VisibleSecret{item.Metadata.Name, item.Metadata.Namespace, item.Metadata.UID, item.Metadata.CreationTimestamp.UTC().Format(time.RFC3339), item.Type, item.DataKeys}
			actual[visible.Namespace+"/"+visible.Name] = visible
			total += len(observation)
		}
		if total > 4096 {
			t.Fatal("all visible observation bound")
		}
		for _, secret := range secrets {
			if namespace != "" && secret.Namespace != namespace {
				continue
			}
			wantVisible := accelerator00Visible(secret)
			if actual[secret.Namespace+"/"+secret.Name] != wantVisible {
				t.Fatal("exact six visible fields")
			}
		}
	}
}

func TestDirectSecretBaselineListCancellation(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(canceled) }))
	defer server.Close()
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal("client")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := (&Client{clientset: cs}).ListSecretsMetadataWithContext(ctx, secretfixtures.NamespaceA)
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if err != ErrRequestCancelled {
			t.Fatal("cancel mapping")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel join")
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("request cancel")
	}
}

func accelerator00Visible(s *corev1.Secret) accelerator00VisibleSecret {
	return accelerator00VisibleSecret{s.Name, s.Namespace, string(s.UID), s.CreationTimestamp.UTC().Format(time.RFC3339), string(s.Type), len(s.Data)}
}

func TestDirectSecretBaselineDetailsAndMutations(t *testing.T) {
	objects := secretfixtures.Secrets()
	// Keep the construction explicit so the test exercises current Direct methods.
	client := newTestClient(objects[1], objects[2], objects[5], objects[6])
	entries, err := client.GetSecretData(secretfixtures.NamespaceA, "text")
	if err != nil || len(entries) != 2 {
		t.Fatal("text detail")
	}
	if encoded, marshalErr := json.Marshal(entries); marshalErr != nil || len(encoded) == 0 || len(encoded) > 2*1024*1024 {
		t.Fatal("text serialized bound")
	}
	for _, entry := range entries {
		got, err := BytesFromDataEntry(entry)
		if err != nil || len(got) == 0 || !bytes.Equal(got, objects[1].Data[entry.Key]) {
			t.Fatal("detail fidelity")
		}
	}
	large, err := client.GetSecretData(secretfixtures.NamespaceB, "large")
	if err != nil || len(large) != 1 {
		t.Fatal("large detail")
	}
	largeBytes, err := BytesFromDataEntry(large[0])
	serialized, marshalErr := json.Marshal(large)
	if err != nil || marshalErr != nil || len(largeBytes) != 512*1024 || len(serialized) <= 512*1024 || len(serialized) > 2*1024*1024 {
		t.Fatal("large detail bound")
	}
	binary, err := client.GetSecretData(secretfixtures.NamespaceA, "binary")
	if err != nil || len(binary) != 1 {
		t.Fatal("binary detail")
	}
	binaryBytes, err := BytesFromDataEntry(binary[0])
	if err != nil || !bytes.Equal(binaryBytes, objects[2].Data[binary[0].Key]) || !binary[0].IsBinary {
		t.Fatal("binary fidelity")
	}
	if encoded, marshalErr := json.Marshal(binary); marshalErr != nil || len(encoded) == 0 || len(encoded) > 2*1024*1024 {
		t.Fatal("binary serialized bound")
	}
	yaml, err := client.GetSecretYaml(secretfixtures.NamespaceB, "yaml")
	var yamlSecret corev1.Secret
	if err != nil || len(yaml) == 0 || len(yaml) > 2*1024*1024 || yamlutil.Unmarshal([]byte(yaml), &yamlSecret) != nil || yamlSecret.Name != objects[5].Name || yamlSecret.Namespace != objects[5].Namespace || yamlSecret.UID != objects[5].UID || yamlSecret.Type != objects[5].Type || len(yamlSecret.Data) != len(objects[5].Data) {
		t.Fatal("yaml detail")
	}
	for key, want := range objects[5].Data {
		if got, ok := yamlSecret.Data[key]; !ok || !bytes.Equal(got, want) {
			t.Fatal("yaml fixture fidelity")
		}
	}
	entries[0].Value = "updated"
	if err := client.UpdateSecretData(secretfixtures.NamespaceA, "text", entries); err != nil {
		t.Fatal("update data")
	}
	if err := client.UpdateSecretYaml(secretfixtures.NamespaceB, "yaml", yaml); err != nil {
		t.Fatal("update yaml")
	}
	updated, err := client.GetSecretData(secretfixtures.NamespaceA, "text")
	updatedByKey := map[string]DataEntry{}
	for _, entry := range updated {
		updatedByKey[entry.Key] = entry
	}
	if err != nil || len(updated) != len(entries) || updatedByKey[entries[0].Key].Value != entries[0].Value {
		t.Fatal("data update effect")
	}
	yamlSecret.Labels = map[string]string{"baseline": "updated"}
	updatedYAMLBytes, err := yamlutil.Marshal(&yamlSecret)
	if err != nil || client.UpdateSecretYaml(secretfixtures.NamespaceB, "yaml", string(updatedYAMLBytes)) != nil {
		t.Fatal("yaml update effect")
	}
	updatedYAML, err := client.GetSecretYaml(secretfixtures.NamespaceB, "yaml")
	var updatedYAMLSecret corev1.Secret
	if err != nil || yamlutil.Unmarshal([]byte(updatedYAML), &updatedYAMLSecret) != nil || updatedYAMLSecret.Labels["baseline"] != "updated" || !bytes.Equal(updatedYAMLSecret.Data["text"], yamlSecret.Data["text"]) || !bytes.Equal(updatedYAMLSecret.Data["bin"], yamlSecret.Data["bin"]) {
		t.Fatal("yaml update fidelity")
	}
	if err := client.DeleteSecret("", secretfixtures.NamespaceA, "text"); err != nil {
		t.Fatal("delete")
	}
	if _, err := client.GetSecretData(secretfixtures.NamespaceA, "text"); err == nil {
		t.Fatal("delete effect")
	}
}

func TestDirectSecretBaselineVisibleFixtures(t *testing.T) {
	secrets := secretfixtures.Secrets()
	counts := map[string]int{}
	var total int
	for _, secret := range secrets {
		counts[secret.Namespace]++
		encoded, err := json.Marshal(accelerator00Visible(secret))
		if err != nil || len(encoded) > 512 {
			t.Fatal("visible observation bound")
		}
		total += len(encoded)
	}
	if counts[secretfixtures.NamespaceA] != 5 || counts[secretfixtures.NamespaceB] != 2 || total > 4096 {
		t.Fatal("visible fixture counts")
	}
	a, b := secrets[0].DeepCopy(), secrets[0].DeepCopy()
	a.Data, b.Data = map[string][]byte{"private-a": []byte("a")}, map[string][]byte{"private-b": []byte("b")}
	a.Labels, b.Labels = map[string]string{"unsafe": "one"}, map[string]string{"unsafe": "two"}
	a.Annotations, b.Annotations = map[string]string{"unsafe": "one"}, map[string]string{"unsafe": "two"}
	if accelerator00Visible(a) != accelerator00Visible(b) || bytes.Equal(a.Data["private-a"], b.Data["private-b"]) {
		t.Fatal("visible projection")
	}
}

func TestDirectSecretBaselineWatchSequence(t *testing.T) {
	state := map[string]accelerator00VisibleSecret{}
	events := secretfixtures.ChurnEvents()
	for i, event := range events {
		key := string(event.UID)
		if i == len(events)-1 {
			delete(state, key)
		} else {
			state[key] = accelerator00Visible(&event)
		}
		if len(state) > 1 {
			t.Fatal("bounded reducer")
		}
	}
	if len(state) != 0 {
		t.Fatal("delete state")
	}
	_ = context.Background()
}

func TestDirectSecretBaselineWatchAndTeardown(t *testing.T) {
	for _, delivery := range []struct {
		name  string
		limit int
	}{{"all events", len(secretfixtures.ChurnEvents())}, {"stopped delivery", 1}} {
		t.Run(delivery.name, func(t *testing.T) {
			fixture := secretfixtures.ChurnEvents()
			ctx, cancel := context.WithCancel(context.Background())
			requested, requestClosed := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api/v1/namespaces/"+secretfixtures.NamespaceA+"/secrets" || r.URL.Query().Get("watch") != "true" {
					t.Error("watch request")
					w.WriteHeader(http.StatusNotFound)
					return
				}
				close(requested)
				w.Header().Set("Content-Type", "application/json")
				encoder := json.NewEncoder(w)
				for i, secret := range fixture[:delivery.limit] {
					object := secret.DeepCopy()
					object.APIVersion, object.Kind = "v1", "Secret"
					typ := "MODIFIED"
					if i == 0 {
						typ = "ADDED"
					}
					if i == len(fixture)-1 {
						typ = "DELETED"
					}
					if encoder.Encode(map[string]any{"type": typ, "object": object}) != nil {
						return
					}
				}
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(requestClosed)
			}))
			// Server.Close waits for handlers. Cancellation must therefore precede it,
			// even when a failure stops delivery before the fixture is exhausted.
			defer func() {
				cancel()
				select {
				case <-requestClosed:
				case <-time.After(time.Second):
					t.Error("watch request cleanup")
				}
				server.Close()
			}()
			cs, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
			if err != nil {
				t.Fatal("watch client")
			}
			client := &Client{clientset: cs}
			stream, err := client.WatchResource(ctx, "secrets", secretfixtures.NamespaceA, "")
			if err != nil {
				t.Fatal("watch start")
			}
			<-requested
			closed := make(chan struct{})
			delivered := make(chan struct{})
			observations := map[string]accelerator00VisibleSecret{}
			go func() {
				defer close(closed)
				count := 0
				for event := range stream.ResultChan() {
					secret, ok := event.Object.(*corev1.Secret)
					if !ok || secret.Namespace != secretfixtures.NamespaceA {
						continue
					}
					if event.Type == watch.Deleted {
						delete(observations, string(secret.UID))
					} else {
						observations[string(secret.UID)] = accelerator00Visible(secret)
					}
					if len(observations) > 1 {
						return
					}
					count++
					if count == delivery.limit {
						close(delivered)
					}
				}
			}()
			select {
			case <-delivered:
			case <-time.After(time.Second):
				t.Fatal("watch delivery")
			}
			cancel()
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("watch join")
			}
			if delivery.limit == len(fixture) && len(observations) != 0 {
				t.Fatal("delete observation")
			}
			select {
			case <-requestClosed:
			case <-time.After(time.Second):
				t.Fatal("watch request cancel")
			}
		})
	}
}
