package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"

	"kubikles/pkg/agent"
	"kubikles/pkg/k8s"
	"kubikles/pkg/server"
)

func acceleratorProjectionTestClient(t *testing.T, handler http.Handler) *k8s.Client {
	t.Helper()
	api := httptest.NewServer(handler)
	t.Cleanup(api.Close)
	client, err := k8s.NewClientForRESTConfig(&rest.Config{Host: api.URL})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestAcceleratorHideHelmOptionExactDefaults(t *testing.T) {
	for _, test := range []struct {
		name string
		args []json.RawMessage
		want bool
		bad  bool
	}{
		{"absent", nil, true, false}, {"nil", []json.RawMessage{nil, nil, nil}, true, false}, {"true", []json.RawMessage{nil, nil, json.RawMessage(`true`)}, true, false}, {"false", []json.RawMessage{nil, nil, json.RawMessage(`false`)}, false, false}, {"null", []json.RawMessage{nil, nil, json.RawMessage(`null`)}, false, false}, {"invalid", []json.RawMessage{nil, nil, json.RawMessage(`"wrong"`)}, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := acceleratorHideHelmOption(test.args)
			if (err != nil) != test.bad || got != test.want {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
}

type acceleratorDelegate struct {
	call   agent.AuthenticatedCallContext
	method string
	args   []json.RawMessage
}

func (d *acceleratorDelegate) CallMethod(call agent.AuthenticatedCallContext, method string, args []json.RawMessage) (interface{}, error) {
	d.call, d.method, d.args = call, method, args
	return "delegated", nil
}

func TestAcceleratorSecretCallerDelegatesOtherMethodsExactly(t *testing.T) {
	delegate := &acceleratorDelegate{}
	caller := newAcceleratorSecretCaller(delegate, &App{})
	call := agent.AuthenticatedCallContext{PrincipalID: "creator", SessionID: "session"}
	args := []json.RawMessage{json.RawMessage(`"value"`)}
	got, err := caller.CallMethod(call, "GetPods", args)
	if err != nil || got != "delegated" || delegate.call != call || delegate.method != "GetPods" || string(delegate.args[0]) != string(args[0]) {
		t.Fatalf("delegate = %#v %v %#v", got, err, delegate)
	}
}

func TestAppMethodCallerDispatchesExactAcceleratorSecretUnsubscribe(t *testing.T) {
	trusted := agent.AuthenticatedCallContext{PrincipalID: "trusted-unsubscribe", SessionID: "session-unsubscribe"}
	watcherSpecID := SecretWatchSpecID("owned-watcher-spec")
	leases := &secretWatchLeaseStore{leases: map[agent.SessionID]server.AcceleratorSessionLease{
		trusted.SessionID: {Generation: 7, Connected: true},
	}}
	manager := &AcceleratorSecretWatchManager{
		streams:            map[SecretWatchSpecID]*secretWatchStream{},
		sessionSpecs:       map[agent.SessionID]map[SecretWatchSpecID]server.AcceleratorSocketGeneration{trusted.SessionID: {watcherSpecID: 7}},
		sessionGenerations: map[agent.SessionID]server.AcceleratorSocketGeneration{trusted.SessionID: 7},
		lease:              leases.lookup,
	}
	caller := NewAppMethodCaller(&App{acceleratorSecretWatches: manager})
	if result, err := caller.CallMethod(trusted, "UnsubscribeSecretWatcher", []json.RawMessage{json.RawMessage(`"owned-watcher-spec"`)}); err != nil || result != nil {
		t.Fatalf("unsubscribe dispatch = %#v, %v", result, err)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if _, found := manager.sessionSpecs[trusted.SessionID]; found {
		t.Fatalf("exact trusted unsubscribe retained membership: %#v", manager.sessionSpecs)
	}
}

func TestAcceleratorSecretListDTOOmitsOrdinaryMetadata(t *testing.T) {
	item := acceleratorSecretListItem{Type: "Opaque", DataKeys: 1}
	item.Metadata.Name = "example"
	item.Metadata.Namespace = "ns"
	item.Metadata.UID = "uid"
	item.Metadata.CreationTimestamp = metav1.Now()
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"labels", "annotations"} {
		if string(data) == "" || containsJSONField(data, forbidden) {
			t.Fatalf("accelerator DTO leaked %q: %s", forbidden, data)
		}
	}
}

func containsJSONField(data []byte, field string) bool {
	var value map[string]interface{}
	_ = json.Unmarshal(data, &value)
	metadata, _ := value["metadata"].(map[string]interface{})
	_, exists := metadata[field]
	return exists
}

func TestAcceleratorSecretDetailMethods(t *testing.T) {
	secret := v1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "detail", Namespace: "evidence", UID: "uid-detail", Labels: map[string]string{"private-label": "label-value"},
			Annotations: map[string]string{"private-annotation": "annotation-value"}, ResourceVersion: "rv-detail",
			Finalizers: []string{"example/finalizer"}, OwnerReferences: []metav1.OwnerReference{{APIVersion: "v1", Kind: "ConfigMap", Name: "owner", UID: "owner-uid"}},
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "MANAGED_FIELDS_FORBIDDEN"}},
		},
		Type:       v1.SecretTypeOpaque,
		Data:       map[string][]byte{"z-binary": {0xff, 0x00, 0x7f}, "a-text": []byte("DETAIL_MARKER")},
		StringData: map[string]string{"never-project": "STRING_DATA_FORBIDDEN"},
	}
	client := acceleratorProjectionTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/namespaces/evidence/secrets/detail" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(secret); err != nil {
			t.Errorf("encode secret: %v", err)
		}
	}))
	caller := newAcceleratorSecretCaller(&acceleratorDelegate{}, &App{k8sClient: client})
	trusted := agent.AuthenticatedCallContext{PrincipalID: "creator-detail", SessionID: "session-detail"}
	args := []json.RawMessage{json.RawMessage(`"evidence"`), json.RawMessage(`"detail"`)}

	result, err := caller.CallMethod(trusted, "GetSecretData", args)
	if err != nil {
		t.Fatal(err)
	}
	entries, ok := result.([]k8s.DataEntry)
	if !ok {
		t.Fatalf("GetSecretData result = %T", result)
	}
	wantEntries := []k8s.DataEntry{
		{Key: "a-text", Value: "DETAIL_MARKER", Base64Value: base64.StdEncoding.EncodeToString([]byte("DETAIL_MARKER")), Source: k8s.DataEntrySourceData, Encoding: k8s.DataEntryEncodingText},
		{Key: "z-binary", Base64Value: "/wB/", IsBinary: true, Source: k8s.DataEntrySourceData, Encoding: k8s.DataEntryEncodingBase64},
	}
	if !reflect.DeepEqual(entries, wantEntries) {
		t.Fatalf("GetSecretData = %#v, want %#v", entries, wantEntries)
	}
	decodedBytes, err := k8s.BytesFromDataEntry(entries[1])
	if err != nil || !reflect.DeepEqual(decodedBytes, []byte{0xff, 0x00, 0x7f}) {
		t.Fatalf("binary bytes = %v, %v", decodedBytes, err)
	}
	entries[0].Value = "mutated"
	second, err := caller.CallMethod(trusted, "GetSecretData", args)
	if err != nil || second.([]k8s.DataEntry)[0].Value != "DETAIL_MARKER" {
		t.Fatalf("GetSecretData clone = %#v, %v", second, err)
	}

	result, err = caller.CallMethod(trusted, "GetSecretYaml", args)
	if err != nil {
		t.Fatal(err)
	}
	yamlText, ok := result.(string)
	if !ok {
		t.Fatalf("GetSecretYaml = %T", result)
	}
	if strings.Contains(yamlText, "managedFields") || strings.Contains(yamlText, "MANAGED_FIELDS_FORBIDDEN") {
		t.Fatalf("GetSecretYaml leaked managed fields: %s", yamlText)
	}
	var decoded v1.Secret
	if err := yaml.Unmarshal([]byte(yamlText), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ResourceVersion != "rv-detail" || decoded.UID != "uid-detail" || decoded.Labels["private-label"] != "label-value" || decoded.Annotations["private-annotation"] != "annotation-value" || len(decoded.Finalizers) != 1 || len(decoded.OwnerReferences) != 1 || decoded.Type != v1.SecretTypeOpaque || !reflect.DeepEqual(decoded.Data, secret.Data) {
		t.Fatalf("GetSecretYaml lost Direct-compatible fields: %#v", decoded)
	}
}

func TestAcceleratorSecretDataPreservesEmptyResult(t *testing.T) {
	client := acceleratorProjectionTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/namespaces/evidence/secrets/empty" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "empty", Namespace: "evidence"}, Data: map[string][]byte{}}); err != nil {
			t.Errorf("encode secret: %v", err)
		}
	}))
	caller := newAcceleratorSecretCaller(&acceleratorDelegate{}, &App{k8sClient: client})
	result, err := caller.CallMethod(agent.AuthenticatedCallContext{PrincipalID: "creator-empty", SessionID: "session-empty"}, "GetSecretData", []json.RawMessage{json.RawMessage(`"evidence"`), json.RawMessage(`"empty"`)})
	if err != nil {
		t.Fatal(err)
	}
	entries, ok := result.([]k8s.DataEntry)
	if !ok || entries == nil || len(entries) != 0 {
		t.Fatalf("GetSecretData empty result = %#v", result)
	}
}

func TestAcceleratorSecretListArguments(t *testing.T) {
	type observedRequest struct {
		path     string
		selector string
	}
	requests := make(chan observedRequest, 16)
	client := acceleratorProjectionTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- observedRequest{path: r.URL.Path, selector: r.URL.Query().Get("fieldSelector")}
		table := metav1.Table{ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name"}, {Name: "Type"}, {Name: "Data"}}, Rows: []metav1.TableRow{{Cells: []interface{}{"safe", "Opaque", float64(0)}}}}
		if err := json.NewEncoder(w).Encode(table); err != nil {
			t.Errorf("encode table: %v", err)
		}
	}))
	caller := newAcceleratorSecretCaller(&acceleratorDelegate{}, &App{k8sClient: client, listRequestManager: NewListRequestManager()})
	trusted := agent.AuthenticatedCallContext{PrincipalID: "creator-list", SessionID: "session-list"}
	tests := []struct {
		name         string
		args         []json.RawMessage
		wantPath     string
		wantSelector bool
	}{
		{name: "two arguments defaults true", args: []json.RawMessage{json.RawMessage(`"request-absent"`), json.RawMessage(`"evidence"`)}, wantPath: "/api/v1/namespaces/evidence/secrets", wantSelector: true},
		{name: "explicit true", args: []json.RawMessage{json.RawMessage(`"request-true"`), json.RawMessage(`"evidence"`), json.RawMessage(`true`)}, wantPath: "/api/v1/namespaces/evidence/secrets", wantSelector: true},
		{name: "explicit false", args: []json.RawMessage{json.RawMessage(`"request-false"`), json.RawMessage(`"evidence"`), json.RawMessage(`false`)}, wantPath: "/api/v1/namespaces/evidence/secrets", wantSelector: false},
		{name: "nil defaults true", args: []json.RawMessage{json.RawMessage(`"request-nil"`), json.RawMessage(`"evidence"`), nil}, wantPath: "/api/v1/namespaces/evidence/secrets", wantSelector: true},
		{name: "JSON null is false", args: []json.RawMessage{json.RawMessage(`"request-null"`), json.RawMessage(`"evidence"`), json.RawMessage(`null`)}, wantPath: "/api/v1/namespaces/evidence/secrets", wantSelector: false},
		{name: "missing strings use zero values", args: nil, wantPath: "/api/v1/secrets", wantSelector: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := caller.CallMethod(trusted, "ListSecretsMetadata", test.args)
			if err != nil {
				t.Fatal(err)
			}
			items, ok := result.([]acceleratorSecretListItem)
			if !ok || len(items) != 1 || items[0].Metadata.Name != "safe" {
				t.Fatalf("result = %T %#v", result, result)
			}
			observed := <-requests
			if observed.path != test.wantPath || (observed.selector != "") != test.wantSelector {
				t.Fatalf("request = %#v, want path %q selector=%v", observed, test.wantPath, test.wantSelector)
			}
		})
	}
	before := len(requests)
	_, err := caller.CallMethod(trusted, "ListSecretsMetadata", []json.RawMessage{json.RawMessage(`"request-bad"`), json.RawMessage(`"evidence"`), json.RawMessage(`"malformed"`)})
	if err == nil || !strings.Contains(err.Error(), "argument 2") {
		t.Fatalf("malformed bool error = %v", err)
	}
	if len(requests) != before {
		t.Fatal("malformed bool reached Kubernetes client")
	}
}

func TestAcceleratorSecretProjectionCallerDelegatesWithTrustedContext(t *testing.T) {
	trusted := agent.AuthenticatedCallContext{PrincipalID: "trusted-principal", SessionID: "trusted-session"}
	for _, method := range []string{"CancelListRequest", "UnknownFutureMethod", "ListSecrets", "UpdateSecretData", "UpdateSecretYaml", "DeleteSecret"} {
		t.Run(method, func(t *testing.T) {
			delegate := &acceleratorDelegate{}
			caller := newAcceleratorSecretCaller(delegate, &App{})
			args := []json.RawMessage{json.RawMessage(`{"PrincipalID":"forged-principal","SessionID":"forged-session"}`), json.RawMessage(`"raw-value"`)}
			result, err := caller.CallMethod(trusted, method, args)
			if err != nil || result != "delegated" {
				t.Fatalf("CallMethod = %#v, %v", result, err)
			}
			if delegate.call != trusted || delegate.method != method || !reflect.DeepEqual(delegate.args, args) || &delegate.args[0] != &args[0] {
				t.Fatalf("delegated call = %#v %q %#v", delegate.call, delegate.method, delegate.args)
			}
			if delegate.call.PrincipalID == "forged-principal" || delegate.call.SessionID == "forged-session" {
				t.Fatalf("forged identity influenced trusted context: %#v", delegate.call)
			}
		})
	}
}

func TestOrdinarySecretDetailNonRegression(t *testing.T) {
	secret := v1.Secret{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{Name: "ordinary", Namespace: "evidence", ResourceVersion: "ordinary-rv", Labels: map[string]string{"ordinary-label": "LABEL_MARKER"}, Annotations: map[string]string{"ordinary-annotation": "ANNOTATION_MARKER"}, Finalizers: []string{"FINALIZER_MARKER"}},
		Type:       v1.SecretTypeOpaque, Data: map[string][]byte{"ordinary-data": []byte("DATA_MARKER")}, StringData: map[string]string{"ordinary-string": "STRING_MARKER"},
	}
	client := acceleratorProjectionTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/namespaces/evidence/secrets" {
			table := metav1.Table{ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name"}, {Name: "Type"}, {Name: "Data"}}, Rows: []metav1.TableRow{
				{Cells: []interface{}{"ordinary", "Opaque", float64(1)}, Object: runtime.RawExtension{Raw: []byte(`{"metadata":{"namespace":"evidence","labels":{"ordinary-label":"LABEL_MARKER"},"annotations":{"ordinary-annotation":"ANNOTATION_MARKER"}}}`)}},
				{Cells: []interface{}{"sh.helm.release.v1.app.v1", k8s.HelmReleaseSecretType, float64(1)}},
			}}
			_ = json.NewEncoder(w).Encode(table)
			return
		}
		if r.URL.Path == "/api/v1/namespaces/evidence/secrets/ordinary" {
			_ = json.NewEncoder(w).Encode(secret)
			return
		}
		http.NotFound(w, r)
	}))
	app := &App{k8sClient: client, listRequestManager: NewListRequestManager()}
	items, err := app.ListSecretsMetadata("", "evidence")
	if err != nil || len(items) != 2 || items[0].Metadata.Labels["ordinary-label"] != "LABEL_MARKER" || items[0].Metadata.Annotations["ordinary-annotation"] != "ANNOTATION_MARKER" || items[1].Type != k8s.HelmReleaseSecretType {
		t.Fatalf("ordinary list = %#v, %v", items, err)
	}
	data, err := app.GetSecretData("evidence", "ordinary")
	if err != nil || len(data) != 1 || data[0].Value != "DATA_MARKER" {
		t.Fatalf("ordinary data = %#v, %v", data, err)
	}
	yaml, err := app.GetSecretYaml("evidence", "ordinary")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"LABEL_MARKER", "ANNOTATION_MARKER", "ordinary-rv", "FINALIZER_MARKER", "STRING_MARKER", "REFUQV9NQVJLRVI="} {
		if !strings.Contains(yaml, marker) {
			t.Fatalf("ordinary YAML omitted %q: %s", marker, yaml)
		}
	}
}
