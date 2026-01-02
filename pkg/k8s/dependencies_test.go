package k8s

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func boolPtr(b bool) *bool {
	return &b
}

func stringPtr(s string) *string {
	return &s
}

func TestGetPodDependencies(t *testing.T) {
	// Create a pod with owner reference and service account
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       "test-rs",
					Controller: boolPtr(true),
				},
			},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "test-sa",
			Containers: []corev1.Container{
				{Name: "main", Image: "nginx"},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	// Create a service that selects the pod
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-svc",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "test"},
		},
	}

	// Create the fake client
	cs := fake.NewSimpleClientset(pod, svc)

	client := &Client{}
	graph := &DependencyGraph{Nodes: []DependencyNode{}, Edges: []DependencyEdge{}}
	nodeMap := make(map[string]bool)

	result, err := client.getPodDependencies(cs, "default", "test-pod", graph, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify nodes
	nodeKinds := make(map[string]bool)
	for _, node := range result.Nodes {
		nodeKinds[node.Kind] = true
	}

	if !nodeKinds["Pod"] {
		t.Error("expected Pod node")
	}
	if !nodeKinds["ReplicaSet"] {
		t.Error("expected ReplicaSet node from owner reference")
	}
	if !nodeKinds["ServiceAccount"] {
		t.Error("expected ServiceAccount node")
	}
	if !nodeKinds["Service"] {
		t.Error("expected Service node")
	}

	// Verify edges
	edgeRelations := make(map[string]bool)
	for _, edge := range result.Edges {
		edgeRelations[edge.Relation] = true
	}

	if !edgeRelations["owns"] {
		t.Error("expected 'owns' edge from ReplicaSet to Pod")
	}
	if !edgeRelations["uses"] {
		t.Error("expected 'uses' edge from Pod to ServiceAccount")
	}
	if !edgeRelations["selects"] {
		t.Error("expected 'selects' edge from Service to Pod")
	}
}

func TestGetPodDependencies_DefaultServiceAccount(t *testing.T) {
	// Pod with default service account should not show ServiceAccount node
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "default",
			Containers: []corev1.Container{
				{Name: "main", Image: "nginx"},
			},
		},
	}

	cs := fake.NewSimpleClientset(pod)
	client := &Client{}
	graph := &DependencyGraph{Nodes: []DependencyNode{}, Edges: []DependencyEdge{}}
	nodeMap := make(map[string]bool)

	result, err := client.getPodDependencies(cs, "default", "test-pod", graph, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, node := range result.Nodes {
		if node.Kind == "ServiceAccount" {
			t.Error("should not include default ServiceAccount in graph")
		}
	}
}

func TestGetDeploymentDependencies(t *testing.T) {
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy",
			Namespace: "default",
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy-abc123",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "Deployment", Name: "test-deploy", Controller: boolPtr(true)},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deploy-abc123-xyz",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "test-deploy-abc123", Controller: boolPtr(true)},
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "test"}},
	}

	cs := fake.NewSimpleClientset(deploy, rs, pod, svc)
	client := &Client{}
	graph := &DependencyGraph{Nodes: []DependencyNode{}, Edges: []DependencyEdge{}}
	nodeMap := make(map[string]bool)

	result, err := client.getDeploymentDependencies(cs, "", "default", "test-deploy", graph, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nodeKinds := make(map[string]bool)
	for _, node := range result.Nodes {
		nodeKinds[node.Kind] = true
	}

	if !nodeKinds["Deployment"] {
		t.Error("expected Deployment node")
	}
	if !nodeKinds["ReplicaSet"] {
		t.Error("expected ReplicaSet node")
	}
	if !nodeKinds["Pod"] {
		t.Error("expected Pod node")
	}
	if !nodeKinds["Service"] {
		t.Error("expected Service node")
	}
}

func TestGetServiceDependencies(t *testing.T) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "test-svc", Namespace: "default"},
		Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "test"}},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ingress", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			IngressClassName: stringPtr("nginx"),
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: "/",
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: "test-svc",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	cs := fake.NewSimpleClientset(svc, pod, ingress)
	client := &Client{}
	graph := &DependencyGraph{Nodes: []DependencyNode{}, Edges: []DependencyEdge{}}
	nodeMap := make(map[string]bool)

	result, err := client.getServiceDependencies(cs, "", "default", "test-svc", graph, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nodeKinds := make(map[string]bool)
	for _, node := range result.Nodes {
		nodeKinds[node.Kind] = true
	}

	if !nodeKinds["Service"] {
		t.Error("expected Service node")
	}
	if !nodeKinds["Pod"] {
		t.Error("expected Pod node")
	}
	if !nodeKinds["Ingress"] {
		t.Error("expected Ingress node")
	}
	if !nodeKinds["IngressClass"] {
		t.Error("expected IngressClass node")
	}

	// Check edge relations
	hasRoutesTo := false
	for _, edge := range result.Edges {
		if edge.Relation == "routes-to" {
			hasRoutesTo = true
		}
	}
	if !hasRoutesTo {
		t.Error("expected 'routes-to' edge from Ingress to Service")
	}
}

func TestGetIngressDependencies(t *testing.T) {
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "test-ingress", Namespace: "default"},
		Spec: networkingv1.IngressSpec{
			IngressClassName: stringPtr("nginx"),
			TLS: []networkingv1.IngressTLS{
				{SecretName: "tls-secret"},
			},
			Rules: []networkingv1.IngressRule{
				{
					Host: "example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{Name: "test-svc"},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	cs := fake.NewSimpleClientset(ingress)
	client := &Client{}
	graph := &DependencyGraph{Nodes: []DependencyNode{}, Edges: []DependencyEdge{}}
	nodeMap := make(map[string]bool)

	result, err := client.getIngressDependencies(cs, "", "default", "test-ingress", graph, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nodeKinds := make(map[string]bool)
	for _, node := range result.Nodes {
		nodeKinds[node.Kind] = true
	}

	if !nodeKinds["Ingress"] {
		t.Error("expected Ingress node")
	}
	if !nodeKinds["IngressClass"] {
		t.Error("expected IngressClass node")
	}
	if !nodeKinds["Secret"] {
		t.Error("expected Secret node for TLS")
	}
	if !nodeKinds["Service"] {
		t.Error("expected Service node")
	}
}

func TestGetHPADependencies(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: "test-hpa", Namespace: "default"},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "test-deploy",
			},
		},
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "test-deploy", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	cs := fake.NewSimpleClientset(hpa, deploy)
	client := &Client{}
	graph := &DependencyGraph{Nodes: []DependencyNode{}, Edges: []DependencyEdge{}}
	nodeMap := make(map[string]bool)

	result, err := client.getHPADependencies(cs, "", "default", "test-hpa", graph, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nodeKinds := make(map[string]bool)
	for _, node := range result.Nodes {
		nodeKinds[node.Kind] = true
	}

	if !nodeKinds["HorizontalPodAutoscaler"] {
		t.Error("expected HorizontalPodAutoscaler node")
	}
	if !nodeKinds["Deployment"] {
		t.Error("expected Deployment node")
	}

	// Check for scales edge
	hasScales := false
	for _, edge := range result.Edges {
		if edge.Relation == "scales" {
			hasScales = true
		}
	}
	if !hasScales {
		t.Error("expected 'scales' edge from HPA to Deployment")
	}
}

func TestGetPDBDependencies(t *testing.T) {
	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pdb", Namespace: "default"},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	cs := fake.NewSimpleClientset(pdb, pod)
	client := &Client{}
	graph := &DependencyGraph{Nodes: []DependencyNode{}, Edges: []DependencyEdge{}}
	nodeMap := make(map[string]bool)

	result, err := client.getPDBDependencies(cs, "", "default", "test-pdb", graph, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nodeKinds := make(map[string]bool)
	for _, node := range result.Nodes {
		nodeKinds[node.Kind] = true
	}

	if !nodeKinds["PodDisruptionBudget"] {
		t.Error("expected PodDisruptionBudget node")
	}
	if !nodeKinds["Pod"] {
		t.Error("expected Pod node")
	}

	// Check for protects edge
	hasProtects := false
	for _, edge := range result.Edges {
		if edge.Relation == "protects" {
			hasProtects = true
		}
	}
	if !hasProtects {
		t.Error("expected 'protects' edge from PDB to Pod")
	}
}

func TestGetConfigMapDependencies(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
		Data:       map[string]string{"key": "value"},
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels:    map[string]string{"app": "test"},
		},
		Spec: corev1.PodSpec{
			Volumes: []corev1.Volume{
				{
					Name: "config",
					VolumeSource: corev1.VolumeSource{
						ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "test-cm"},
						},
					},
				},
			},
			Containers: []corev1.Container{{Name: "main", Image: "nginx"}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	cs := fake.NewSimpleClientset(cm, pod)
	client := &Client{}
	graph := &DependencyGraph{Nodes: []DependencyNode{}, Edges: []DependencyEdge{}}
	nodeMap := make(map[string]bool)

	result, err := client.getConfigMapDependencies(cs, "", "default", "test-cm", graph, nodeMap)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	nodeKinds := make(map[string]bool)
	for _, node := range result.Nodes {
		nodeKinds[node.Kind] = true
	}

	if !nodeKinds["ConfigMap"] {
		t.Error("expected ConfigMap node")
	}
	if !nodeKinds["Pod"] {
		t.Error("expected Pod node")
	}
}

func TestMatchesSelector(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		selector map[string]string
		expected bool
	}{
		{
			name:     "exact match",
			labels:   map[string]string{"app": "test", "env": "prod"},
			selector: map[string]string{"app": "test"},
			expected: true,
		},
		{
			name:     "no match",
			labels:   map[string]string{"app": "other"},
			selector: map[string]string{"app": "test"},
			expected: false,
		},
		{
			name:     "empty selector matches all",
			labels:   map[string]string{"app": "test"},
			selector: map[string]string{},
			expected: true,
		},
		{
			name:     "missing label",
			labels:   map[string]string{"app": "test"},
			selector: map[string]string{"app": "test", "env": "prod"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesSelector(tt.labels, tt.selector)
			if result != tt.expected {
				t.Errorf("matchesSelector(%v, %v) = %v, want %v", tt.labels, tt.selector, result, tt.expected)
			}
		})
	}
}

func TestNodeID(t *testing.T) {
	tests := []struct {
		kind      string
		namespace string
		name      string
		expected  string
	}{
		{"Pod", "default", "test", "Pod/default/test"},
		{"PersistentVolume", "", "pv-1", "PersistentVolume/pv-1"},
		{"StorageClass", "", "standard", "StorageClass/standard"},
	}

	for _, tt := range tests {
		result := nodeID(tt.kind, tt.namespace, tt.name)
		if result != tt.expected {
			t.Errorf("nodeID(%s, %s, %s) = %s, want %s", tt.kind, tt.namespace, tt.name, result, tt.expected)
		}
	}
}

func TestAddNodeDeduplication(t *testing.T) {
	client := &Client{}
	graph := &DependencyGraph{Nodes: []DependencyNode{}, Edges: []DependencyEdge{}}
	nodeMap := make(map[string]bool)

	node := DependencyNode{ID: "Pod/default/test", Kind: "Pod", Name: "test", Namespace: "default"}

	// Add same node twice
	client.addNode(graph, nodeMap, node)
	client.addNode(graph, nodeMap, node)

	if len(graph.Nodes) != 1 {
		t.Errorf("expected 1 node after deduplication, got %d", len(graph.Nodes))
	}
}

func TestAggregateGraphStorageClass(t *testing.T) {
	// Create a graph for StorageClass with many PVs
	// The edge direction is: PV → StorageClass (uses)
	graph := &DependencyGraph{
		Nodes: []DependencyNode{
			{ID: "StorageClass/standard", Kind: "StorageClass", Name: "standard"},
			{ID: "PersistentVolume/pv-1", Kind: "PersistentVolume", Name: "pv-1"},
			{ID: "PersistentVolume/pv-2", Kind: "PersistentVolume", Name: "pv-2"},
			{ID: "PersistentVolume/pv-3", Kind: "PersistentVolume", Name: "pv-3"},
			{ID: "PersistentVolume/pv-4", Kind: "PersistentVolume", Name: "pv-4"},
			{ID: "PersistentVolume/pv-5", Kind: "PersistentVolume", Name: "pv-5"},
			{ID: "PersistentVolume/pv-6", Kind: "PersistentVolume", Name: "pv-6"},
			{ID: "PersistentVolume/pv-7", Kind: "PersistentVolume", Name: "pv-7"},
			{ID: "PersistentVolumeClaim/default/pvc-1", Kind: "PersistentVolumeClaim", Name: "pvc-1", Namespace: "default"},
			{ID: "PersistentVolumeClaim/default/pvc-2", Kind: "PersistentVolumeClaim", Name: "pvc-2", Namespace: "default"},
		},
		Edges: []DependencyEdge{
			// PVs use StorageClass (child → parent direction)
			{Source: "PersistentVolume/pv-1", Target: "StorageClass/standard", Relation: "uses"},
			{Source: "PersistentVolume/pv-2", Target: "StorageClass/standard", Relation: "uses"},
			{Source: "PersistentVolume/pv-3", Target: "StorageClass/standard", Relation: "uses"},
			{Source: "PersistentVolume/pv-4", Target: "StorageClass/standard", Relation: "uses"},
			{Source: "PersistentVolume/pv-5", Target: "StorageClass/standard", Relation: "uses"},
			{Source: "PersistentVolume/pv-6", Target: "StorageClass/standard", Relation: "uses"},
			{Source: "PersistentVolume/pv-7", Target: "StorageClass/standard", Relation: "uses"},
			// PVCs bind to PVs
			{Source: "PersistentVolumeClaim/default/pvc-1", Target: "PersistentVolume/pv-1", Relation: "binds"},
			{Source: "PersistentVolumeClaim/default/pvc-2", Target: "PersistentVolume/pv-2", Relation: "binds"},
		},
	}

	// Aggregate with limit of 5 PVs, StorageClass as root
	result := aggregateGraph(graph, 5, "StorageClass/standard")

	// Verify StorageClass is kept as root
	scFound := false
	for _, node := range result.Nodes {
		if node.ID == "StorageClass/standard" {
			scFound = true
			break
		}
	}
	if !scFound {
		t.Error("StorageClass should be present as root")
	}

	// Count PVs (should be limited to 5 + summary)
	pvCount := 0
	summaryFound := false
	for _, node := range result.Nodes {
		if node.Kind == "PersistentVolume" && !node.IsSummary {
			pvCount++
		}
		if node.IsSummary && node.Kind == "PersistentVolume" {
			summaryFound = true
			if node.RemainingCount != 2 {
				t.Errorf("expected summary to have 2 remaining, got %d", node.RemainingCount)
			}
		}
	}
	if pvCount > 5 {
		t.Errorf("expected at most 5 PVs, got %d", pvCount)
	}
	if !summaryFound {
		t.Error("summary node should be created for excess PVs")
	}

	// Verify pv-1 and pv-2 are kept (they have PVCs bound - connectors)
	pv1Found := false
	pv2Found := false
	for _, node := range result.Nodes {
		if node.ID == "PersistentVolume/pv-1" {
			pv1Found = true
		}
		if node.ID == "PersistentVolume/pv-2" {
			pv2Found = true
		}
	}
	if !pv1Found {
		t.Error("pv-1 should be kept (has PVC bound)")
	}
	if !pv2Found {
		t.Error("pv-2 should be kept (has PVC bound)")
	}
}

func TestAggregateGraphPreservesConnectors(t *testing.T) {
	// Create a graph where Pod2 connects to both ReplicaSet (owns) and Service (selects)
	// This pod should NOT be aggregated away even if there are many pods
	graph := &DependencyGraph{
		Nodes: []DependencyNode{
			{ID: "Deployment/default/nginx", Kind: "Deployment", Name: "nginx", Namespace: "default"},
			{ID: "ReplicaSet/default/nginx-abc", Kind: "ReplicaSet", Name: "nginx-abc", Namespace: "default"},
			{ID: "Pod/default/pod-1", Kind: "Pod", Name: "pod-1", Namespace: "default"},
			{ID: "Pod/default/pod-2", Kind: "Pod", Name: "pod-2", Namespace: "default"}, // Connector - selected by Service
			{ID: "Pod/default/pod-3", Kind: "Pod", Name: "pod-3", Namespace: "default"},
			{ID: "Pod/default/pod-4", Kind: "Pod", Name: "pod-4", Namespace: "default"},
			{ID: "Pod/default/pod-5", Kind: "Pod", Name: "pod-5", Namespace: "default"},
			{ID: "Pod/default/pod-6", Kind: "Pod", Name: "pod-6", Namespace: "default"},
			{ID: "Pod/default/pod-7", Kind: "Pod", Name: "pod-7", Namespace: "default"},
			{ID: "Service/default/nginx-svc", Kind: "Service", Name: "nginx-svc", Namespace: "default"},
			{ID: "Ingress/default/nginx-ing", Kind: "Ingress", Name: "nginx-ing", Namespace: "default"},
		},
		Edges: []DependencyEdge{
			{Source: "Deployment/default/nginx", Target: "ReplicaSet/default/nginx-abc", Relation: "owns"},
			{Source: "ReplicaSet/default/nginx-abc", Target: "Pod/default/pod-1", Relation: "owns"},
			{Source: "ReplicaSet/default/nginx-abc", Target: "Pod/default/pod-2", Relation: "owns"},
			{Source: "ReplicaSet/default/nginx-abc", Target: "Pod/default/pod-3", Relation: "owns"},
			{Source: "ReplicaSet/default/nginx-abc", Target: "Pod/default/pod-4", Relation: "owns"},
			{Source: "ReplicaSet/default/nginx-abc", Target: "Pod/default/pod-5", Relation: "owns"},
			{Source: "ReplicaSet/default/nginx-abc", Target: "Pod/default/pod-6", Relation: "owns"},
			{Source: "ReplicaSet/default/nginx-abc", Target: "Pod/default/pod-7", Relation: "owns"},
			// Service selects pod-2 - this makes pod-2 a connector node
			{Source: "Service/default/nginx-svc", Target: "Pod/default/pod-2", Relation: "selects"},
			{Source: "Ingress/default/nginx-ing", Target: "Service/default/nginx-svc", Relation: "routes-to"},
		},
	}

	// Aggregate with limit of 5 pods
	result := aggregateGraph(graph, 5, "Deployment/default/nginx")

	// Verify pod-2 is kept (it's a connector to Service)
	pod2Found := false
	for _, node := range result.Nodes {
		if node.ID == "Pod/default/pod-2" {
			pod2Found = true
			break
		}
	}
	if !pod2Found {
		t.Error("pod-2 should be kept as it's a connector node (selected by Service)")
	}

	// Verify Service and Ingress are present and connected
	serviceFound := false
	ingressFound := false
	for _, node := range result.Nodes {
		if node.ID == "Service/default/nginx-svc" {
			serviceFound = true
		}
		if node.ID == "Ingress/default/nginx-ing" {
			ingressFound = true
		}
	}
	if !serviceFound {
		t.Error("Service should be present in aggregated graph")
	}
	if !ingressFound {
		t.Error("Ingress should be present in aggregated graph")
	}

	// Verify there's an edge from Service to pod-2
	serviceToPodfound := false
	for _, edge := range result.Edges {
		if edge.Source == "Service/default/nginx-svc" && edge.Target == "Pod/default/pod-2" {
			serviceToPodfound = true
			break
		}
	}
	if !serviceToPodfound {
		t.Error("Edge from Service to pod-2 should be preserved")
	}

	// Verify there's a summary node for excess pods
	summaryFound := false
	for _, node := range result.Nodes {
		if node.IsSummary && node.Kind == "Pod" {
			summaryFound = true
			break
		}
	}
	if !summaryFound {
		t.Error("Summary node should be created for excess pods")
	}
}

func TestSplitSummaryID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "valid summary ID with namespace",
			input:    "summary:Deployment/default/nginx:Pod",
			expected: []string{"summary", "Deployment/default/nginx", "Pod"},
		},
		{
			name:     "valid summary ID without namespace",
			input:    "summary:StorageClass/standard:PersistentVolume",
			expected: []string{"summary", "StorageClass/standard", "PersistentVolume"},
		},
		{
			name:     "invalid - too short",
			input:    "short",
			expected: nil,
		},
		{
			name:     "invalid - no prefix",
			input:    "notasummary:Deployment/default/nginx:Pod",
			expected: nil,
		},
		{
			name:     "invalid - no kind separator",
			input:    "summary:Deployment/default/nginx",
			expected: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := splitSummaryID(tc.input)
			if tc.expected == nil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
				return
			}
			if len(result) != len(tc.expected) {
				t.Errorf("expected %d parts, got %d: %v", len(tc.expected), len(result), result)
				return
			}
			for i, exp := range tc.expected {
				if result[i] != exp {
					t.Errorf("part %d: expected %q, got %q", i, exp, result[i])
				}
			}
		})
	}
}
