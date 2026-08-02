//go:build !headless && helm && !accelerator

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"kubikles/pkg/acceleratorprovision"
	"kubikles/pkg/acceleratorrelease"
	"kubikles/pkg/agent"
	"kubikles/pkg/events"
	"kubikles/pkg/helm"
	"kubikles/pkg/k8s"
)

type recordingDesktopAcceleratorCoordinator struct {
	mu       sync.Mutex
	events   []string
	onFence  func(string)
	onNotify func(string, bool)
}

func (c *recordingDesktopAcceleratorCoordinator) record(event string) {
	c.mu.Lock()
	c.events = append(c.events, event)
	c.mu.Unlock()
}
func (c *recordingDesktopAcceleratorCoordinator) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.events...)
}
func (c *recordingDesktopAcceleratorCoordinator) AcquireSecretDemand(context.Context, string) acceleratorprovision.DemandResult {
	return acceleratorprovision.DemandResult{Reason: acceleratorprovision.DemandRuntimeClosing}
}
func (c *recordingDesktopAcceleratorCoordinator) FenceContextSwitch(name string) {
	c.record("fence:" + name)
	if c.onFence != nil {
		c.onFence(name)
	}
}
func (c *recordingDesktopAcceleratorCoordinator) ContextSwitched(name string, success bool) {
	if success {
		c.record("switched:" + name)
	} else {
		c.record("failed:" + name)
	}
	if c.onNotify != nil {
		c.onNotify(name, success)
	}
}
func (c *recordingDesktopAcceleratorCoordinator) Quiesce(context.Context) {
	c.record("accelerator-quiesce")
}
func (c *recordingDesktopAcceleratorCoordinator) StopProducers(context.Context) {
	c.record("accelerator-stop")
}
func (c *recordingDesktopAcceleratorCoordinator) Close(context.Context) {
	c.record("accelerator-close")
}

type orderedRootLifecycle struct {
	coordinator *recordingDesktopAcceleratorCoordinator
}

func (l orderedRootLifecycle) Quiesce(context.Context)       { l.coordinator.record("next-quiesce") }
func (l orderedRootLifecycle) StopProducers(context.Context) { l.coordinator.record("next-stop") }
func (l orderedRootLifecycle) Close(context.Context)         { l.coordinator.record("next-close") }

func TestDesktopAcceleratorConstructionIsDormantAndModeIsolated(t *testing.T) {
	originalConfigDir := desktopUserConfigDir
	originalResolver := desktopAcceleratorReleaseResolverFactory
	originalProvisioner := desktopAcceleratorProvisionerFactory
	originalConnector := desktopAcceleratorConnectorFactory
	originalReconnector := desktopAcceleratorReconnectorFactory
	originalDisposer := desktopAcceleratorDisposerFactory
	originalCoordinator := desktopAcceleratorCoordinatorFactory
	t.Cleanup(func() {
		desktopUserConfigDir = originalConfigDir
		desktopAcceleratorReleaseResolverFactory = originalResolver
		desktopAcceleratorProvisionerFactory = originalProvisioner
		desktopAcceleratorConnectorFactory = originalConnector
		desktopAcceleratorReconnectorFactory = originalReconnector
		desktopAcceleratorDisposerFactory = originalDisposer
		desktopAcceleratorCoordinatorFactory = originalCoordinator
	})
	desktopUserConfigDir = func() (string, error) { return t.TempDir(), nil }
	calls := []string{}
	desktopAcceleratorReleaseResolverFactory = func(version, root string) *acceleratorrelease.Resolver {
		calls = append(calls, "resolver")
		return acceleratorrelease.New(version, root)
	}
	desktopAcceleratorProvisionerFactory = func(*k8s.Client, *helm.Client) *acceleratorprovision.Service {
		calls = append(calls, "provisioner")
		return nil
	}
	desktopAcceleratorConnectorFactory = func(string) *acceleratorprovision.Connector {
		calls = append(calls, "connector")
		return nil
	}
	desktopAcceleratorReconnectorFactory = func(string) *acceleratorprovision.Reconnector {
		calls = append(calls, "reconnector")
		return nil
	}
	desktopAcceleratorDisposerFactory = func(*acceleratorprovision.Service) *acceleratorprovision.DisposalService {
		calls = append(calls, "disposer")
		return nil
	}
	created := &recordingDesktopAcceleratorCoordinator{}
	desktopAcceleratorCoordinatorFactory = func(*k8s.Client, *acceleratorrelease.Resolver, *acceleratorprovision.Service, *acceleratorprovision.Connector, *acceleratorprovision.Reconnector, *acceleratorprovision.DisposalService) desktopAcceleratorCoordinator {
		calls = append(calls, "coordinator")
		return created
	}

	profile := appConstructionProfile{
		newListRequestManager: NewListRequestManager,
		initializeOrdinaryServices: func(app *App) {
			app.helmClient = helm.NewClient()
		},
	}
	app, err := newAppWithOptionsAndProfile(AppOptions{Mode: RuntimeModeDesktop, KubernetesClientFactory: func() (*k8s.Client, error) { return &k8s.Client{}, nil }}, profile)
	if err != nil {
		t.Fatal(err)
	}
	if app.acceleratorLifecycle != created || !reflect.DeepEqual(calls, []string{"resolver", "provisioner", "connector", "reconnector", "disposer", "coordinator"}) {
		t.Fatalf("desktop lifecycle=%T calls=%v", app.acceleratorLifecycle, calls)
	}
	if _, ok := app.lifecycle.(chainedRuntimeLifecycle); !ok {
		t.Fatalf("desktop lifecycle was not bound through RuntimeLifecycle: %T", app.lifecycle)
	}
	for _, mode := range []RuntimeMode{RuntimeModeServer} {
		calls = nil
		isolated, createErr := newAppWithOptionsAndProfile(AppOptions{Mode: mode, KubernetesClientFactory: func() (*k8s.Client, error) { return &k8s.Client{}, nil }}, profile)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if len(calls) != 0 {
			t.Fatalf("mode %s constructed Accelerator services: %v", mode, calls)
		}
		if _, ok := isolated.acceleratorLifecycle.(directOnlyAcceleratorCoordinator); !ok {
			t.Fatalf("mode %s lifecycle=%T", mode, isolated.acceleratorLifecycle)
		}
	}
}

func TestSwitchContextFencesOldAcceleratorBeforeKubernetesSwitch(t *testing.T) {
	client := newContextSwitchTestClient(t)
	recorder := &recordingDesktopAcceleratorCoordinator{}
	recorder.onFence = func(name string) {
		if got := client.GetCurrentContext(); got != name {
			t.Fatalf("fence observed current=%q name=%q", got, name)
		}
	}
	app := &App{k8sClient: client, acceleratorLifecycle: recorder}
	if err := app.SwitchContext("new"); err != nil {
		t.Fatal(err)
	}
	if got := client.GetCurrentContext(); got != "new" {
		t.Fatalf("current=%q", got)
	}
	if got := recorder.snapshot(); !reflect.DeepEqual(got, []string{"fence:old", "switched:new"}) {
		t.Fatalf("events=%v", got)
	}
	if err := app.SwitchContext("missing"); err == nil {
		t.Fatal("missing context switch succeeded")
	}
	if got := recorder.snapshot(); !reflect.DeepEqual(got, []string{"fence:old", "switched:new", "fence:new", "failed:missing"}) {
		t.Fatalf("failure events=%v", got)
	}
}

func TestConcurrentContextMutationsSerializeFenceMutateNotify(t *testing.T) {
	client := newContextSwitchTestClient(t)
	recorder := &recordingDesktopAcceleratorCoordinator{}
	app := &App{k8sClient: client, acceleratorLifecycle: recorder}

	const rounds = 50
	for round := 0; round < rounds; round++ {
		fenceEntered, releaseFence := make(chan struct{}), make(chan struct{})
		lockViolation := make(chan struct{}, 1)
		assertTransactionLocked := func() {
			if app.contextMutationMu.TryLock() {
				app.contextMutationMu.Unlock()
				select {
				case lockViolation <- struct{}{}:
				default:
				}
			}
		}
		var firstFence sync.Once
		recorder.onFence = func(string) {
			assertTransactionLocked()
			firstFence.Do(func() {
				close(fenceEntered)
				<-releaseFence
			})
		}
		recorder.onNotify = func(string, bool) { assertTransactionLocked() }
		firstDone := make(chan error, 1)
		secondDone := make(chan error, 1)
		secondStarted := make(chan struct{})
		go func() { firstDone <- app.SwitchContext("new") }()
		<-fenceEntered
		go func() {
			close(secondStarted)
			secondDone <- app.SwitchContext("old")
		}()
		<-secondStarted
		close(releaseFence)
		if err := <-firstDone; err != nil {
			t.Fatalf("round %d first switch: %v", round, err)
		}
		if err := <-secondDone; err != nil {
			t.Fatalf("round %d second switch: %v", round, err)
		}
		wantSuffix := []string{"fence:old", "switched:new", "fence:new", "switched:old"}
		got := recorder.snapshot()
		if suffix := got[len(got)-len(wantSuffix):]; !reflect.DeepEqual(suffix, wantSuffix) {
			t.Fatalf("round %d transaction order=%v", round, suffix)
		}
		if current := client.GetCurrentContext(); current != "old" {
			t.Fatalf("round %d current=%q", round, current)
		}
		select {
		case <-lockViolation:
			t.Fatalf("round %d fence/mutate/notify transaction was not mutex-held", round)
		default:
		}
	}
}

func TestActiveContextMutationsFenceAcceleratorAuthority(t *testing.T) {
	t.Run("rename success and failure", func(t *testing.T) {
		client := newContextSwitchTestClient(t)
		recorder := &recordingDesktopAcceleratorCoordinator{}
		recorder.onFence = func(name string) {
			if got := client.GetCurrentContext(); got != name {
				t.Fatalf("rename fence observed current=%q name=%q", got, name)
			}
		}
		app := &App{k8sClient: client, acceleratorLifecycle: recorder}
		if err := app.RenameContext("old", "renamed"); err != nil {
			t.Fatal(err)
		}
		if got := recorder.snapshot(); !reflect.DeepEqual(got, []string{"fence:old", "switched:renamed"}) {
			t.Fatalf("rename events=%v", got)
		}
		if err := app.RenameContext("renamed", "new"); err == nil {
			t.Fatal("rename onto existing context succeeded")
		}
		if got := recorder.snapshot(); !reflect.DeepEqual(got, []string{"fence:old", "switched:renamed", "fence:renamed", "failed:new"}) {
			t.Fatalf("failed rename events=%v", got)
		}
	})

	t.Run("detail update", func(t *testing.T) {
		client := newContextSwitchTestClient(t)
		recorder := &recordingDesktopAcceleratorCoordinator{}
		recorder.onFence = func(name string) {
			detail, err := client.GetFullContextDetail(name)
			if err != nil || detail.Namespace != "" {
				t.Fatalf("update fence ran after mutation: detail=%#v err=%v", detail, err)
			}
		}
		app := &App{k8sClient: client, acceleratorLifecycle: recorder}
		namespace := "accelerated"
		if err := app.UpdateContextDetail("old", k8s.ContextUpdateRequest{Namespace: &namespace}); err != nil {
			t.Fatal(err)
		}
		if got := recorder.snapshot(); !reflect.DeepEqual(got, []string{"fence:old", "switched:old"}) {
			t.Fatalf("update events=%v", got)
		}
	})

	t.Run("extra kubeconfig paths", func(t *testing.T) {
		client := newContextSwitchTestClient(t)
		recorder := &recordingDesktopAcceleratorCoordinator{}
		recorder.onFence = func(name string) {
			if got := client.GetCurrentContext(); got != name {
				t.Fatalf("paths fence observed current=%q name=%q", got, name)
			}
		}
		app := &App{k8sClient: client, acceleratorLifecycle: recorder}
		app.SetExtraKubeconfigPaths([]string{filepath.Join(t.TempDir(), "extra-config")})
		if got := recorder.snapshot(); !reflect.DeepEqual(got, []string{"fence:old", "switched:old"}) {
			t.Fatalf("paths events=%v", got)
		}
	})
}

func TestCoordinatorLeavesRepresentativeDesktopOperationsDirect(t *testing.T) {
	var calls []string
	secret := v1.Secret{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"}, ObjectMeta: metav1.ObjectMeta{Name: "ordinary", Namespace: "evidence"}, Data: map[string][]byte{"key": []byte("value")}}
	configMap := v1.ConfigMap{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"}, ObjectMeta: metav1.ObjectMeta{Name: "ordinary", Namespace: "evidence"}, Data: map[string]string{"key": "value"}}
	client := acceleratorProjectionTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls = append(calls, request.Method+" "+request.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/api/v1/namespaces/evidence/secrets":
			_ = json.NewEncoder(w).Encode(metav1.Table{ColumnDefinitions: []metav1.TableColumnDefinition{{Name: "Name"}, {Name: "Type"}, {Name: "Data"}}, Rows: []metav1.TableRow{{Cells: []interface{}{"ordinary", "Opaque", float64(1)}, Object: runtime.RawExtension{Raw: []byte(`{"metadata":{"namespace":"evidence"}}`)}}}})
		case "/api/v1/namespaces/evidence/secrets/ordinary":
			_ = json.NewEncoder(w).Encode(secret)
		case "/api/v1/namespaces/evidence/configmaps/ordinary":
			_ = json.NewEncoder(w).Encode(configMap)
		default:
			http.NotFound(w, request)
		}
	}))
	recorder := &recordingDesktopAcceleratorCoordinator{}
	app := &App{
		k8sClient: client, listRequestManager: NewListRequestManager(), agentRouter: NoopAgentRouter{}, acceleratorLifecycle: recorder,
		emitter: events.EmitterFunc(func(string, ...interface{}) { t.Fatal("coordinator emitted a UI event") }),
	}
	if app.agentRouter.Supports(agent.CapabilitySecretsList) {
		t.Fatal("40F changed the no-op AgentRouter")
	}
	if items, err := app.ListSecretsMetadata("", "evidence"); err != nil || len(items) != 1 {
		t.Fatalf("direct Secret list=%#v err=%v", items, err)
	}
	if data, err := app.GetSecretData("evidence", "ordinary"); err != nil || len(data) != 1 || data[0].Value != "value" {
		t.Fatalf("direct Secret data=%#v err=%v", data, err)
	}
	if yaml, err := app.GetSecretYaml("evidence", "ordinary"); err != nil || yaml == "" {
		t.Fatalf("direct Secret YAML=%q err=%v", yaml, err)
	}
	if data, err := app.GetConfigMapData("evidence", "ordinary"); err != nil || len(data) != 1 || data[0].Value != "value" {
		t.Fatalf("direct non-Secret data=%#v err=%v", data, err)
	}
	if err := app.UpdateSecretData("evidence", "ordinary", nil); err != nil {
		t.Fatalf("direct Secret mutation: %v", err)
	}
	if got := recorder.snapshot(); len(got) != 0 {
		t.Fatalf("direct operations entered coordinator lifecycle: %v", got)
	}
	if len(calls) < 6 {
		t.Fatalf("representative operations did not reach Kubernetes directly: %v", calls)
	}
}

func TestDesktopAcceleratorRuntimeLifecycleShutdownOrder(t *testing.T) {
	recorder := &recordingDesktopAcceleratorCoordinator{}
	app := &App{runtimeMode: RuntimeModeDesktop, acceleratorLifecycle: recorder, lifecycle: chainedRuntimeLifecycle{accelerator: recorder, next: orderedRootLifecycle{coordinator: recorder}}}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	app.runShutdownPhases(cancelled)
	app.runShutdownPhases(context.Background())
	want := []string{"accelerator-quiesce", "next-quiesce", "accelerator-stop", "next-stop", "accelerator-close", "next-close"}
	if got := recorder.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("shutdown=%v want=%v", got, want)
	}
}

func newContextSwitchTestClient(t *testing.T) *k8s.Client {
	t.Helper()
	home := t.TempDir()
	configDir := filepath.Join(home, ".kube")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	config := `apiVersion: v1
kind: Config
current-context: old
clusters:
- name: old
  cluster: {server: http://127.0.0.1:1}
- name: new
  cluster: {server: http://127.0.0.1:1}
contexts:
- name: old
  context: {cluster: old, user: test}
- name: new
  context: {cluster: new, user: test}
users:
- name: test
  user: {}
`
	if err := os.WriteFile(filepath.Join(configDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	client, err := k8s.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	client.SetAPITimeout(10 * time.Millisecond)
	return client
}
