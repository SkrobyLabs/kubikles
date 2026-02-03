package k8s

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	schedulingv1 "k8s.io/api/scheduling/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// Helper to create a fake client with objects
func newTestClient(objects ...runtime.Object) *Client {
	cs := fake.NewSimpleClientset(objects...)
	return &Client{
		clientset:      cs,
		currentContext: "test-context",
	}
}

// ============================================================================
// Context and Configuration Tests
// ============================================================================

func TestGetCurrentContext(t *testing.T) {
	client := &Client{currentContext: "my-context"}
	if got := client.GetCurrentContext(); got != "my-context" {
		t.Errorf("GetCurrentContext() = %q, want %q", got, "my-context")
	}
}

func TestGetCurrentContext_Empty(t *testing.T) {
	client := &Client{}
	if got := client.GetCurrentContext(); got != "" {
		t.Errorf("GetCurrentContext() = %q, want empty string", got)
	}
}

// ============================================================================
// Pod Tests
// ============================================================================

func TestListPods(t *testing.T) {
	pods := []runtime.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c1", Image: "nginx"}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "default"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c1", Image: "nginx"}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-other", Namespace: "other"},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c1", Image: "nginx"}}},
		},
	}
	client := newTestClient(pods...)

	result, err := client.ListPods("default")
	if err != nil {
		t.Fatalf("ListPods() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListPods() returned %d pods, want 2", len(result))
	}
}

func TestListPods_AllNamespaces(t *testing.T) {
	pods := []runtime.Object{
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "ns1"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "ns2"}},
	}
	client := newTestClient(pods...)

	result, err := client.ListPods("")
	if err != nil {
		t.Fatalf("ListPods() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListPods('') returned %d pods, want 2", len(result))
	}
}

func TestListPods_EmptyNamespace(t *testing.T) {
	client := newTestClient()
	result, err := client.ListPods("nonexistent")
	if err != nil {
		t.Fatalf("ListPods() error = %v", err)
	}
	if len(result) != 0 {
		t.Errorf("ListPods() returned %d pods, want 0", len(result))
	}
}

// ============================================================================
// Node Tests
// ============================================================================

func TestListNodes(t *testing.T) {
	nodes := []runtime.Object{
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-1"}},
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-2"}},
	}
	client := newTestClient(nodes...)

	result, err := client.ListNodes()
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListNodes() returned %d nodes, want 2", len(result))
	}
}

func TestGetNodeYaml(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Spec:       corev1.NodeSpec{Unschedulable: false},
	}
	client := newTestClient(node)

	yaml, err := client.GetNodeYaml("test-node")
	if err != nil {
		t.Fatalf("GetNodeYaml() error = %v", err)
	}
	if !strings.Contains(yaml, "test-node") {
		t.Error("GetNodeYaml() should contain node name")
	}
	// Note: fake client doesn't populate TypeMeta, so we check for metadata instead
	if !strings.Contains(yaml, "metadata:") {
		t.Error("GetNodeYaml() should contain metadata section")
	}
}

func TestGetNodeYaml_NotFound(t *testing.T) {
	client := newTestClient()
	_, err := client.GetNodeYaml("nonexistent")
	if err == nil {
		t.Error("GetNodeYaml() should return error for nonexistent node")
	}
}

// ============================================================================
// Namespace Tests
// ============================================================================

func TestListNamespaces(t *testing.T) {
	namespaces := []runtime.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "app"}},
	}
	client := newTestClient(namespaces...)

	result, err := client.ListNamespaces()
	if err != nil {
		t.Fatalf("ListNamespaces() error = %v", err)
	}
	if len(result) != 3 {
		t.Errorf("ListNamespaces() returned %d namespaces, want 3", len(result))
	}
}

func TestGetNamespaceResourceCounts(t *testing.T) {
	objects := []runtime.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "test-ns"}},
		// Pods
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "test-ns"}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-2", Namespace: "test-ns"}},
		// Services
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-1", Namespace: "test-ns"}},
		// Deployments
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "deploy-1", Namespace: "test-ns"}},
		// ConfigMaps
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm-1", Namespace: "test-ns"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm-2", Namespace: "test-ns"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm-3", Namespace: "test-ns"}},
		// Secrets
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret-1", Namespace: "test-ns"}},
	}
	client := newTestClient(objects...)

	result, err := client.GetNamespaceResourceCounts("test-ns")
	if err != nil {
		t.Fatalf("GetNamespaceResourceCounts() error = %v", err)
	}

	if result.Pods != 2 {
		t.Errorf("Pods count = %d, want 2", result.Pods)
	}
	if result.Services != 1 {
		t.Errorf("Services count = %d, want 1", result.Services)
	}
	if result.Deployments != 1 {
		t.Errorf("Deployments count = %d, want 1", result.Deployments)
	}
	if result.ConfigMaps != 3 {
		t.Errorf("ConfigMaps count = %d, want 3", result.ConfigMaps)
	}
	if result.Secrets != 1 {
		t.Errorf("Secrets count = %d, want 1", result.Secrets)
	}
}

func TestGetNamespaceYAML(t *testing.T) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-ns",
			Labels: map[string]string{"env": "test"},
		},
	}
	client := newTestClient(ns)

	yaml, err := client.GetNamespaceYAML("test-ns")
	if err != nil {
		t.Fatalf("GetNamespaceYAML() error = %v", err)
	}
	if !strings.Contains(yaml, "test-ns") {
		t.Error("GetNamespaceYAML() should contain namespace name")
	}
}

func TestDeleteNamespace(t *testing.T) {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "delete-me"}}
	client := newTestClient(ns)

	err := client.DeleteNamespace("test-context", "delete-me")
	if err != nil {
		t.Fatalf("DeleteNamespace() error = %v", err)
	}

	// Verify deletion
	_, err = client.clientset.CoreV1().Namespaces().Get(context.TODO(), "delete-me", metav1.GetOptions{})
	if err == nil {
		t.Error("Namespace should be deleted")
	}
}

// ============================================================================
// Service Tests
// ============================================================================

func TestListServices(t *testing.T) {
	services := []runtime.Object{
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-1", Namespace: "default"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-2", Namespace: "default"}},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "svc-other", Namespace: "other"}},
	}
	client := newTestClient(services...)

	result, err := client.ListServices("default")
	if err != nil {
		t.Fatalf("ListServices() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListServices() returned %d services, want 2", len(result))
	}
}

func TestGetServiceYaml(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{{Port: 80}},
		},
	}
	client := newTestClient(svc)

	yaml, err := client.GetServiceYaml("default", "test-svc")
	if err != nil {
		t.Fatalf("GetServiceYaml() error = %v", err)
	}
	if !strings.Contains(yaml, "test-svc") {
		t.Error("GetServiceYaml() should contain service name")
	}
}

// ============================================================================
// Deployment Tests
// ============================================================================

func TestListDeployments(t *testing.T) {
	deployments := []runtime.Object{
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "deploy-1", Namespace: "default"}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "deploy-2", Namespace: "default"}},
	}
	client := newTestClient(deployments...)

	result, err := client.ListDeployments("default")
	if err != nil {
		t.Fatalf("ListDeployments() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListDeployments() returned %d deployments, want 2", len(result))
	}
}

func TestGetDeploymentYaml(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deploy", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "test"}},
		},
	}
	client := newTestClient(deploy)

	yaml, err := client.GetDeploymentYaml("default", "test-deploy")
	if err != nil {
		t.Fatalf("GetDeploymentYaml() error = %v", err)
	}
	if !strings.Contains(yaml, "test-deploy") {
		t.Error("GetDeploymentYaml() should contain deployment name")
	}
}

func TestDeleteDeployment(t *testing.T) {
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "delete-me", Namespace: "default"}}
	client := newTestClient(deploy)

	err := client.DeleteDeployment("", "default", "delete-me")
	if err != nil {
		t.Fatalf("DeleteDeployment() error = %v", err)
	}
}

// ============================================================================
// StatefulSet Tests
// ============================================================================

func TestListStatefulSets(t *testing.T) {
	statefulsets := []runtime.Object{
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "sts-1", Namespace: "default"}},
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "sts-2", Namespace: "default"}},
	}
	client := newTestClient(statefulsets...)

	result, err := client.ListStatefulSets("", "default")
	if err != nil {
		t.Fatalf("ListStatefulSets() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListStatefulSets() returned %d statefulsets, want 2", len(result))
	}
}

func TestGetStatefulSetYaml(t *testing.T) {
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test-sts", Namespace: "default"},
	}
	client := newTestClient(sts)

	yaml, err := client.GetStatefulSetYaml("default", "test-sts")
	if err != nil {
		t.Fatalf("GetStatefulSetYaml() error = %v", err)
	}
	if !strings.Contains(yaml, "test-sts") {
		t.Error("GetStatefulSetYaml() should contain statefulset name")
	}
}

// ============================================================================
// DaemonSet Tests
// ============================================================================

func TestListDaemonSets(t *testing.T) {
	daemonsets := []runtime.Object{
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "ds-1", Namespace: "default"}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "ds-2", Namespace: "default"}},
	}
	client := newTestClient(daemonsets...)

	result, err := client.ListDaemonSets("", "default")
	if err != nil {
		t.Fatalf("ListDaemonSets() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListDaemonSets() returned %d daemonsets, want 2", len(result))
	}
}

func TestGetDaemonSetYaml(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ds", Namespace: "default"},
	}
	client := newTestClient(ds)

	yaml, err := client.GetDaemonSetYaml("default", "test-ds")
	if err != nil {
		t.Fatalf("GetDaemonSetYaml() error = %v", err)
	}
	if !strings.Contains(yaml, "test-ds") {
		t.Error("GetDaemonSetYaml() should contain daemonset name")
	}
}

// ============================================================================
// ReplicaSet Tests
// ============================================================================

func TestListReplicaSets(t *testing.T) {
	replicasets := []runtime.Object{
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "rs-1", Namespace: "default"}},
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "rs-2", Namespace: "default"}},
	}
	client := newTestClient(replicasets...)

	result, err := client.ListReplicaSets("", "default")
	if err != nil {
		t.Fatalf("ListReplicaSets() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListReplicaSets() returned %d replicasets, want 2", len(result))
	}
}

// ============================================================================
// ConfigMap Tests
// ============================================================================

func TestListConfigMaps(t *testing.T) {
	configmaps := []runtime.Object{
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm-1", Namespace: "default"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm-2", Namespace: "default"}},
	}
	client := newTestClient(configmaps...)

	result, err := client.ListConfigMaps("default")
	if err != nil {
		t.Fatalf("ListConfigMaps() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListConfigMaps() returned %d configmaps, want 2", len(result))
	}
}

func TestGetConfigMapYaml(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}
	client := newTestClient(cm)

	yaml, err := client.GetConfigMapYaml("default", "test-cm")
	if err != nil {
		t.Fatalf("GetConfigMapYaml() error = %v", err)
	}
	if !strings.Contains(yaml, "test-cm") {
		t.Error("GetConfigMapYaml() should contain configmap name")
	}
}

func TestDeleteConfigMap(t *testing.T) {
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "delete-me", Namespace: "default"}}
	client := newTestClient(cm)

	err := client.DeleteConfigMap("test-context", "default", "delete-me")
	if err != nil {
		t.Fatalf("DeleteConfigMap() error = %v", err)
	}
}

// ============================================================================
// Secret Tests
// ============================================================================

func TestListSecrets(t *testing.T) {
	secrets := []runtime.Object{
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret-1", Namespace: "default"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "secret-2", Namespace: "default"}},
	}
	client := newTestClient(secrets...)

	result, err := client.ListSecrets("default")
	if err != nil {
		t.Fatalf("ListSecrets() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListSecrets() returned %d secrets, want 2", len(result))
	}
}

func TestGetSecretData(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-secret", Namespace: "default"},
		Data:       map[string][]byte{"password": []byte("secret123")},
	}
	client := newTestClient(secret)

	data, err := client.GetSecretData("default", "test-secret")
	if err != nil {
		t.Fatalf("GetSecretData() error = %v", err)
	}
	if data["password"] != "secret123" {
		t.Errorf("GetSecretData() password = %q, want %q", data["password"], "secret123")
	}
}

func TestUpdateSecretData(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-secret", Namespace: "default"},
		Data:       map[string][]byte{"old": []byte("data")},
	}
	client := newTestClient(secret)

	newData := map[string]string{"new": "value"}
	err := client.UpdateSecretData("default", "test-secret", newData)
	if err != nil {
		t.Fatalf("UpdateSecretData() error = %v", err)
	}

	// Verify update
	updated, _ := client.clientset.CoreV1().Secrets("default").Get(context.TODO(), "test-secret", metav1.GetOptions{})
	if string(updated.Data["new"]) != "value" {
		t.Error("UpdateSecretData() did not update secret data")
	}
}

func TestDeleteSecret(t *testing.T) {
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "delete-me", Namespace: "default"}}
	client := newTestClient(secret)

	err := client.DeleteSecret("test-context", "default", "delete-me")
	if err != nil {
		t.Fatalf("DeleteSecret() error = %v", err)
	}
}

// ============================================================================
// Ingress Tests
// ============================================================================

func TestListIngresses(t *testing.T) {
	ingresses := []runtime.Object{
		&networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "ing-1", Namespace: "default"}},
		&networkingv1.Ingress{ObjectMeta: metav1.ObjectMeta{Name: "ing-2", Namespace: "default"}},
	}
	client := newTestClient(ingresses...)

	result, err := client.ListIngresses("default")
	if err != nil {
		t.Fatalf("ListIngresses() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListIngresses() returned %d ingresses, want 2", len(result))
	}
}

func TestGetIngressYaml(t *testing.T) {
	ing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ing", Namespace: "default"},
	}
	client := newTestClient(ing)

	yaml, err := client.GetIngressYaml("default", "test-ing")
	if err != nil {
		t.Fatalf("GetIngressYaml() error = %v", err)
	}
	if !strings.Contains(yaml, "test-ing") {
		t.Error("GetIngressYaml() should contain ingress name")
	}
}

// ============================================================================
// Event Tests
// ============================================================================

func TestListEvents(t *testing.T) {
	events := []runtime.Object{
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "event-1", Namespace: "default"}},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "event-2", Namespace: "default"}},
	}
	client := newTestClient(events...)

	result, err := client.ListEvents("default")
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListEvents() returned %d events, want 2", len(result))
	}
}

func TestGetEventYAML(t *testing.T) {
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{Name: "test-event", Namespace: "default"},
		Reason:     "TestReason",
		Message:    "Test message",
	}
	client := newTestClient(event)

	yaml, err := client.GetEventYAML("default", "test-event")
	if err != nil {
		t.Fatalf("GetEventYAML() error = %v", err)
	}
	if !strings.Contains(yaml, "test-event") {
		t.Error("GetEventYAML() should contain event name")
	}
}

func TestDeleteEvent(t *testing.T) {
	event := &corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "delete-me", Namespace: "default"}}
	client := newTestClient(event)

	err := client.DeleteEvent("test-context", "default", "delete-me")
	if err != nil {
		t.Fatalf("DeleteEvent() error = %v", err)
	}
}

// ============================================================================
// Job Tests
// ============================================================================

func TestListJobs(t *testing.T) {
	jobs := []runtime.Object{
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job-1", Namespace: "default"}},
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job-2", Namespace: "default"}},
	}
	client := newTestClient(jobs...)

	result, err := client.ListJobs("", "default")
	if err != nil {
		t.Fatalf("ListJobs() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListJobs() returned %d jobs, want 2", len(result))
	}
}

// ============================================================================
// CronJob Tests
// ============================================================================

func TestListCronJobs(t *testing.T) {
	cronjobs := []runtime.Object{
		&batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: "cj-1", Namespace: "default"}},
		&batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: "cj-2", Namespace: "default"}},
	}
	client := newTestClient(cronjobs...)

	result, err := client.ListCronJobs("", "default")
	if err != nil {
		t.Fatalf("ListCronJobs() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListCronJobs() returned %d cronjobs, want 2", len(result))
	}
}

// ============================================================================
// PersistentVolume Tests
// ============================================================================

func TestListPVs(t *testing.T) {
	pvs := []runtime.Object{
		&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-1"}},
		&corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: "pv-2"}},
	}
	client := newTestClient(pvs...)

	result, err := client.ListPVs("")
	if err != nil {
		t.Fatalf("ListPVs() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListPVs() returned %d pvs, want 2", len(result))
	}
}

// ============================================================================
// PersistentVolumeClaim Tests
// ============================================================================

func TestListPVCs(t *testing.T) {
	pvcs := []runtime.Object{
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-1", Namespace: "default"}},
		&corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "pvc-2", Namespace: "default"}},
	}
	client := newTestClient(pvcs...)

	result, err := client.ListPVCs("", "default")
	if err != nil {
		t.Fatalf("ListPVCs() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListPVCs() returned %d pvcs, want 2", len(result))
	}
}

// ============================================================================
// StorageClass Tests
// ============================================================================

func TestListStorageClasses(t *testing.T) {
	scs := []runtime.Object{
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "standard"}},
		&storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: "fast"}},
	}
	client := newTestClient(scs...)

	result, err := client.ListStorageClasses("")
	if err != nil {
		t.Fatalf("ListStorageClasses() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListStorageClasses() returned %d storageclasses, want 2", len(result))
	}
}

// ============================================================================
// HPA Tests
// ============================================================================

func TestListHPAs(t *testing.T) {
	hpas := []runtime.Object{
		&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "hpa-1", Namespace: "default"}},
		&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "hpa-2", Namespace: "default"}},
	}
	client := newTestClient(hpas...)

	result, err := client.ListHPAs("default")
	if err != nil {
		t.Fatalf("ListHPAs() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListHPAs() returned %d hpas, want 2", len(result))
	}
}

// ============================================================================
// PodDisruptionBudget Tests
// ============================================================================

func TestListPDBs(t *testing.T) {
	pdbs := []runtime.Object{
		&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "pdb-1", Namespace: "default"}},
		&policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Name: "pdb-2", Namespace: "default"}},
	}
	client := newTestClient(pdbs...)

	result, err := client.ListPDBs("default")
	if err != nil {
		t.Fatalf("ListPDBs() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListPDBs() returned %d pdbs, want 2", len(result))
	}
}

// ============================================================================
// NetworkPolicy Tests
// ============================================================================

func TestListNetworkPolicies(t *testing.T) {
	netpols := []runtime.Object{
		&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "np-1", Namespace: "default"}},
		&networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "np-2", Namespace: "default"}},
	}
	client := newTestClient(netpols...)

	result, err := client.ListNetworkPolicies("default")
	if err != nil {
		t.Fatalf("ListNetworkPolicies() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListNetworkPolicies() returned %d networkpolicies, want 2", len(result))
	}
}

// ============================================================================
// ServiceAccount Tests
// ============================================================================

func TestListServiceAccounts(t *testing.T) {
	sas := []runtime.Object{
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "sa-1", Namespace: "default"}},
		&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "sa-2", Namespace: "default"}},
	}
	client := newTestClient(sas...)

	result, err := client.ListServiceAccounts("default")
	if err != nil {
		t.Fatalf("ListServiceAccounts() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListServiceAccounts() returned %d serviceaccounts, want 2", len(result))
	}
}

// ============================================================================
// Role Tests
// ============================================================================

func TestListRoles(t *testing.T) {
	roles := []runtime.Object{
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "role-1", Namespace: "default"}},
		&rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "role-2", Namespace: "default"}},
	}
	client := newTestClient(roles...)

	result, err := client.ListRoles("default")
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListRoles() returned %d roles, want 2", len(result))
	}
}

// ============================================================================
// RoleBinding Tests
// ============================================================================

func TestListRoleBindings(t *testing.T) {
	rbs := []runtime.Object{
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "rb-1", Namespace: "default"}},
		&rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "rb-2", Namespace: "default"}},
	}
	client := newTestClient(rbs...)

	result, err := client.ListRoleBindings("default")
	if err != nil {
		t.Fatalf("ListRoleBindings() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListRoleBindings() returned %d rolebindings, want 2", len(result))
	}
}

// ============================================================================
// ClusterRole Tests
// ============================================================================

func TestListClusterRoles(t *testing.T) {
	crs := []runtime.Object{
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "cr-1"}},
		&rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "cr-2"}},
	}
	client := newTestClient(crs...)

	result, err := client.ListClusterRoles()
	if err != nil {
		t.Fatalf("ListClusterRoles() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListClusterRoles() returned %d clusterroles, want 2", len(result))
	}
}

// ============================================================================
// ClusterRoleBinding Tests
// ============================================================================

func TestListClusterRoleBindings(t *testing.T) {
	crbs := []runtime.Object{
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "crb-1"}},
		&rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "crb-2"}},
	}
	client := newTestClient(crbs...)

	result, err := client.ListClusterRoleBindings()
	if err != nil {
		t.Fatalf("ListClusterRoleBindings() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListClusterRoleBindings() returned %d clusterrolebindings, want 2", len(result))
	}
}

// ============================================================================
// PriorityClass Tests
// ============================================================================

func TestListPriorityClasses(t *testing.T) {
	pcs := []runtime.Object{
		&schedulingv1.PriorityClass{ObjectMeta: metav1.ObjectMeta{Name: "high"}},
		&schedulingv1.PriorityClass{ObjectMeta: metav1.ObjectMeta{Name: "low"}},
	}
	client := newTestClient(pcs...)

	result, err := client.ListPriorityClasses()
	if err != nil {
		t.Fatalf("ListPriorityClasses() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListPriorityClasses() returned %d priorityclasses, want 2", len(result))
	}
}

// ============================================================================
// ResourceQuota Tests
// ============================================================================

func TestListResourceQuotas(t *testing.T) {
	rqs := []runtime.Object{
		&corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "rq-1", Namespace: "default"}},
		&corev1.ResourceQuota{ObjectMeta: metav1.ObjectMeta{Name: "rq-2", Namespace: "default"}},
	}
	client := newTestClient(rqs...)

	result, err := client.ListResourceQuotas("default")
	if err != nil {
		t.Fatalf("ListResourceQuotas() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListResourceQuotas() returned %d resourcequotas, want 2", len(result))
	}
}

// ============================================================================
// LimitRange Tests
// ============================================================================

func TestListLimitRanges(t *testing.T) {
	lrs := []runtime.Object{
		&corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "lr-1", Namespace: "default"}},
		&corev1.LimitRange{ObjectMeta: metav1.ObjectMeta{Name: "lr-2", Namespace: "default"}},
	}
	client := newTestClient(lrs...)

	result, err := client.ListLimitRanges("default")
	if err != nil {
		t.Fatalf("ListLimitRanges() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListLimitRanges() returned %d limitranges, want 2", len(result))
	}
}

// ============================================================================
// Lease Tests
// ============================================================================

func TestListLeases(t *testing.T) {
	leases := []runtime.Object{
		&coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Name: "lease-1", Namespace: "default"}},
		&coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Name: "lease-2", Namespace: "default"}},
	}
	client := newTestClient(leases...)

	result, err := client.ListLeases("", "default")
	if err != nil {
		t.Fatalf("ListLeases() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListLeases() returned %d leases, want 2", len(result))
	}
}

// ============================================================================
// EndpointSlice Tests
// ============================================================================

func TestListEndpointSlices(t *testing.T) {
	eps := []runtime.Object{
		&discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: "eps-1", Namespace: "default"}},
		&discoveryv1.EndpointSlice{ObjectMeta: metav1.ObjectMeta{Name: "eps-2", Namespace: "default"}},
	}
	client := newTestClient(eps...)

	result, err := client.ListEndpointSlices("default")
	if err != nil {
		t.Fatalf("ListEndpointSlices() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListEndpointSlices() returned %d endpointslices, want 2", len(result))
	}
}

// ============================================================================
// Endpoints Tests
// ============================================================================

func TestListEndpoints(t *testing.T) {
	endpoints := []runtime.Object{
		&corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "ep-1", Namespace: "default"}},
		&corev1.Endpoints{ObjectMeta: metav1.ObjectMeta{Name: "ep-2", Namespace: "default"}},
	}
	client := newTestClient(endpoints...)

	result, err := client.ListEndpoints("default")
	if err != nil {
		t.Fatalf("ListEndpoints() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("ListEndpoints() returned %d endpoints, want 2", len(result))
	}
}

// ============================================================================
// RuntimeObjectToMap Tests
// ============================================================================

func TestRuntimeObjectToMap(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c1", Image: "nginx"}}},
	}

	result, err := RuntimeObjectToMap(pod)
	if err != nil {
		t.Fatalf("RuntimeObjectToMap() error = %v", err)
	}

	metadata, ok := result["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("RuntimeObjectToMap() should have metadata")
	}
	if metadata["name"] != "test-pod" {
		t.Errorf("RuntimeObjectToMap() name = %v, want test-pod", metadata["name"])
	}
}

// ============================================================================
// Watch Tests
// ============================================================================

func TestWatchPods(t *testing.T) {
	client := newTestClient()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	watcher, err := client.WatchPods(ctx, "default")
	if err != nil {
		t.Fatalf("WatchPods() error = %v", err)
	}
	defer watcher.Stop()

	// Just verify we got a watcher
	if watcher == nil {
		t.Error("WatchPods() returned nil watcher")
	}
}

func TestWatchResource(t *testing.T) {
	tests := []struct {
		name         string
		resourceType string
		namespace    string
	}{
		{"pods", "pods", "default"},
		{"services", "services", "default"},
		{"deployments", "deployments", "default"},
		{"configmaps", "configmaps", "default"},
		{"secrets", "secrets", "default"},
		{"namespaces", "namespaces", ""},
		{"nodes", "nodes", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClient()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			watcher, err := client.WatchResource(ctx, tt.resourceType, tt.namespace, "")
			if err != nil {
				t.Fatalf("WatchResource(%s) error = %v", tt.resourceType, err)
			}
			defer watcher.Stop()

			if watcher == nil {
				t.Errorf("WatchResource(%s) returned nil watcher", tt.resourceType)
			}
		})
	}
}

// ============================================================================
// Pod Management Tests
// ============================================================================

func TestDeletePod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "delete-me", Namespace: "default"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c1", Image: "nginx"}}},
	}
	client := newTestClient(pod)

	err := client.DeletePod("", "default", "delete-me")
	if err != nil {
		t.Fatalf("DeletePod() error = %v", err)
	}
}

func TestGetPodYaml(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c1", Image: "nginx"}}},
	}
	client := newTestClient(pod)

	yaml, err := client.GetPodYaml("default", "test-pod")
	if err != nil {
		t.Fatalf("GetPodYaml() error = %v", err)
	}
	if !strings.Contains(yaml, "test-pod") {
		t.Error("GetPodYaml() should contain pod name")
	}
}

// ============================================================================
// Node Management Tests
// ============================================================================

func TestSetNodeSchedulable(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
		Spec:       corev1.NodeSpec{Unschedulable: false},
	}
	client := newTestClient(node)

	// Cordon node
	err := client.SetNodeSchedulable("", "test-node", false)
	if err != nil {
		t.Fatalf("SetNodeSchedulable(false) error = %v", err)
	}

	updated, _ := client.clientset.CoreV1().Nodes().Get(context.TODO(), "test-node", metav1.GetOptions{})
	if !updated.Spec.Unschedulable {
		t.Error("SetNodeSchedulable(false) should set Unschedulable to true")
	}

	// Uncordon node
	err = client.SetNodeSchedulable("", "test-node", true)
	if err != nil {
		t.Fatalf("SetNodeSchedulable(true) error = %v", err)
	}

	updated, _ = client.clientset.CoreV1().Nodes().Get(context.TODO(), "test-node", metav1.GetOptions{})
	if updated.Spec.Unschedulable {
		t.Error("SetNodeSchedulable(true) should set Unschedulable to false")
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

func int32Ptr(i int32) *int32 {
	return &i
}
