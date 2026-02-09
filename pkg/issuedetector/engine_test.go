package issuedetector

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// mockCache creates a ResourceCache with pre-populated data (no K8s client needed).
func mockCache(data map[string]interface{}) *ResourceCache {
	return &ResourceCache{
		data: data,
	}
}

func ptr[T any](v T) *T { return &v }

// ---- Networking Rules Tests ----

func TestNET001_MissingIngressClass(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"ingresses": []networkingv1.Ingress{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ing", Namespace: "default"},
				Spec:       networkingv1.IngressSpec{IngressClassName: ptr("nginx")},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "good-ing", Namespace: "default"},
				Spec:       networkingv1.IngressSpec{IngressClassName: ptr("traefik")},
			},
		},
		"ingressclasses": []networkingv1.IngressClass{
			{ObjectMeta: metav1.ObjectMeta{Name: "traefik"}},
		},
	})

	rule := &ruleNET001{baseRule: baseRule{id: "NET001", severity: SeverityWarning, category: CategoryNetworking}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Resource.Name != "test-ing" {
		t.Errorf("expected finding for test-ing, got %s", findings[0].Resource.Name)
	}
}

func TestNET002_ServiceNoMatchingPods(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"services": []v1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "default"},
				Spec:       v1.ServiceSpec{Selector: map[string]string{"app": "nonexistent"}},
			},
		},
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-1", Namespace: "default", Labels: map[string]string{"app": "other"}},
				Status:     v1.PodStatus{Phase: v1.PodRunning},
			},
		},
	})

	rule := &ruleNET002{baseRule: baseRule{id: "NET002", severity: SeverityWarning, category: CategoryNetworking}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestNET004_IngressBackendMissing(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"ingresses": []networkingv1.Ingress{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "test-ing", Namespace: "default"},
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{
											Path: "/api",
											Backend: networkingv1.IngressBackend{
												Service: &networkingv1.IngressServiceBackend{
													Name: "missing-svc",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		"services": []v1.Service{},
	})

	rule := &ruleNET004{baseRule: baseRule{id: "NET004", severity: SeverityCritical, category: CategoryNetworking}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

// ---- Workload Rules Tests ----

func TestWRK001_NoResourceLimits(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "no-limits", Namespace: "default"},
				Spec: v1.PodSpec{
					Containers: []v1.Container{{Name: "main"}},
				},
				Status: v1.PodStatus{Phase: v1.PodRunning},
			},
		},
	})

	rule := &ruleWRK001{baseRule: baseRule{id: "WRK001", severity: SeverityWarning, category: CategoryWorkloads}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestWRK002_CrashLooping(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "crash-pod", Namespace: "default"},
				Status: v1.PodStatus{
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name:         "main",
							RestartCount: 10,
							State: v1.ContainerState{
								Waiting: &v1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
							},
						},
					},
				},
			},
		},
	})

	rule := &ruleWRK002{baseRule: baseRule{id: "WRK002", severity: SeverityCritical, category: CategoryWorkloads}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestWRK003_ReplicaMismatch(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"deployments": []appsv1.Deployment{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "my-deploy", Namespace: "default"},
				Spec:       appsv1.DeploymentSpec{Replicas: ptr(int32(3))},
				Status:     appsv1.DeploymentStatus{AvailableReplicas: 1},
			},
		},
	})

	rule := &ruleWRK003{baseRule: baseRule{id: "WRK003", severity: SeverityWarning, category: CategoryWorkloads}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestWRK004_HPATargetMissing(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"hpas": []autoscalingv2.HorizontalPodAutoscaler{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "my-hpa", Namespace: "default"},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind: "Deployment",
						Name: "gone-deploy",
					},
				},
			},
		},
		"deployments":  []appsv1.Deployment{},
		"statefulsets": []appsv1.StatefulSet{},
	})

	rule := &ruleWRK004{baseRule: baseRule{id: "WRK004", severity: SeverityWarning, category: CategoryWorkloads}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

// ---- Storage Rules Tests ----

func TestSTR001_OrphanPVC(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pvcs": []v1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "orphan-pvc", Namespace: "default"},
				Status:     v1.PersistentVolumeClaimStatus{Phase: v1.ClaimBound},
			},
		},
		"pods": []v1.Pod{},
	})

	rule := &ruleSTR001{baseRule: baseRule{id: "STR001", severity: SeverityInfo, category: CategoryStorage}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestSTR002_PVReleasedFailed(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pvs": []v1.PersistentVolume{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "released-pv"},
				Status:     v1.PersistentVolumeStatus{Phase: v1.VolumeReleased},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "bound-pv"},
				Status:     v1.PersistentVolumeStatus{Phase: v1.VolumeBound},
			},
		},
	})

	rule := &ruleSTR002{baseRule: baseRule{id: "STR002", severity: SeverityWarning, category: CategoryStorage}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

// ---- Security Rules Tests ----

func TestSEC001_RunningAsRoot(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "root-pod", Namespace: "default"},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:            "main",
							SecurityContext: &v1.SecurityContext{RunAsUser: ptr(int64(0))},
						},
					},
				},
				Status: v1.PodStatus{Phase: v1.PodRunning},
			},
		},
	})

	rule := &ruleSEC001{baseRule: baseRule{id: "SEC001", severity: SeverityWarning, category: CategorySecurity}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestSEC002_DefaultServiceAccount(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "default-sa-pod", Namespace: "default"},
				Spec:       v1.PodSpec{ServiceAccountName: "default"},
				Status:     v1.PodStatus{Phase: v1.PodRunning},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "custom-sa-pod", Namespace: "default"},
				Spec:       v1.PodSpec{ServiceAccountName: "my-sa"},
				Status:     v1.PodStatus{Phase: v1.PodRunning},
			},
		},
	})

	rule := &ruleSEC002{baseRule: baseRule{id: "SEC002", severity: SeverityInfo, category: CategorySecurity}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

// ---- Config Rules Tests ----

func TestCFG002_MissingImageTag(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "latest-pod", Namespace: "default"},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{Name: "main", Image: "nginx:latest"},
					},
				},
				Status: v1.PodStatus{Phase: v1.PodRunning},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "untagged-pod", Namespace: "default"},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{Name: "main", Image: "nginx"},
					},
				},
				Status: v1.PodStatus{Phase: v1.PodRunning},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "pinned-pod", Namespace: "default"},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{Name: "main", Image: "nginx:1.25.3"},
					},
				},
				Status: v1.PodStatus{Phase: v1.PodRunning},
			},
		},
	})

	rule := &ruleCFG002{baseRule: baseRule{id: "CFG002", severity: SeverityWarning, category: CategoryConfig}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings (latest + untagged), got %d", len(findings))
	}
}

// ---- YAML Loader Tests ----

func TestParsePath(t *testing.T) {
	cases := []struct {
		input    string
		expected []string
	}{
		{".metadata.name", []string{"metadata", "name"}},
		{".spec.containers[*].name", []string{"spec", "containers", "[*]", "name"}},
		{".spec.tls[*].secretName", []string{"spec", "tls", "[*]", "secretName"}},
	}

	for _, tc := range cases {
		result := parsePath(tc.input)
		if len(result) != len(tc.expected) {
			t.Errorf("parsePath(%q): expected %v, got %v", tc.input, tc.expected, result)
			continue
		}
		for i, v := range result {
			if v != tc.expected[i] {
				t.Errorf("parsePath(%q)[%d]: expected %q, got %q", tc.input, i, tc.expected[i], v)
			}
		}
	}
}

func TestIsLatestOrUntagged(t *testing.T) {
	cases := []struct {
		image    string
		expected bool
	}{
		{"nginx:latest", true},
		{"nginx", true},
		{"myregistry.io/nginx", true},
		{"nginx:1.25.3", false},
		{"myregistry.io/nginx:v2", false},
		{"nginx@sha256:abc", false},
	}

	for _, tc := range cases {
		result := isLatestOrUntagged(tc.image)
		if result != tc.expected {
			t.Errorf("isLatestOrUntagged(%q): expected %v, got %v", tc.image, tc.expected, result)
		}
	}
}

// ---- Engine Tests ----

func TestScanEngine_ListRules(t *testing.T) {
	engine := NewScanEngine("", nil)
	rules := engine.ListRules()
	if len(rules) != 40 {
		t.Errorf("expected 40 built-in rules, got %d", len(rules))
	}

	// Verify all rules have required fields
	for _, r := range rules {
		if r.ID == "" {
			t.Error("rule has empty ID")
		}
		if r.Name == "" {
			t.Errorf("rule %s has empty Name", r.ID)
		}
		if r.Severity == "" {
			t.Errorf("rule %s has empty Severity", r.ID)
		}
		if r.Category == "" {
			t.Errorf("rule %s has empty Category", r.ID)
		}
		if len(r.Requires) == 0 {
			t.Errorf("rule %s has no required resources", r.ID)
		}
	}
}

func TestScanEngine_FilterByCategory(t *testing.T) {
	engine := NewScanEngine("", nil)
	rules := engine.allRules()

	// Count networking rules
	count := 0
	for _, r := range rules {
		if r.Category() == CategoryNetworking {
			count++
		}
	}
	if count != 9 {
		t.Errorf("expected 9 networking rules, got %d", count)
	}
}

// ---- NET003 Port Mismatch Test ----

func TestNET003_PortMismatch(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"services": []v1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "default"},
				Spec: v1.ServiceSpec{
					Selector: map[string]string{"app": "myapp"},
					Ports: []v1.ServicePort{
						{
							Name:       "http",
							Port:       80,
							TargetPort: intstr.FromInt(9999), // not exposed
						},
					},
				},
			},
		},
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "myapp-pod", Namespace: "default", Labels: map[string]string{"app": "myapp"}},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "main",
							Ports: []v1.ContainerPort{{ContainerPort: 8080}},
						},
					},
				},
				Status: v1.PodStatus{Phase: v1.PodRunning},
			},
		},
	})

	rule := &ruleNET003{baseRule: baseRule{id: "NET003", severity: SeverityWarning, category: CategoryNetworking}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

// ---- NET005 Endpoints Not Ready Test ----

func TestNET005_EndpointsNotReady(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"services": []v1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "default"},
				Spec:       v1.ServiceSpec{Selector: map[string]string{"app": "myapp"}},
			},
		},
		"endpoints": []v1.Endpoints{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "default"},
				Subsets: []v1.EndpointSubset{ //nolint:staticcheck // Endpoints API still used in production clusters
					{
						NotReadyAddresses: []v1.EndpointAddress{{IP: "10.0.0.1"}},
					},
				},
			},
		},
	})

	rule := &ruleNET005{baseRule: baseRule{id: "NET005", severity: SeverityWarning, category: CategoryNetworking}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestNET006_DuplicateIngressHost(t *testing.T) {
	prefix := networkingv1.PathTypePrefix

	t.Run("same host same path conflicts", func(t *testing.T) {
		cache := mockCache(map[string]interface{}{
			"ingresses": []networkingv1.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ing-a", Namespace: "default"},
					Spec: networkingv1.IngressSpec{
						IngressClassName: ptr("nginx"),
						Rules: []networkingv1.IngressRule{{
							Host: "app.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{{Path: "/", PathType: &prefix}},
							}},
						}},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "ing-b", Namespace: "default"},
					Spec: networkingv1.IngressSpec{
						IngressClassName: ptr("nginx"),
						Rules: []networkingv1.IngressRule{{
							Host: "app.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{{Path: "/", PathType: &prefix}},
							}},
						}},
					},
				},
			},
		})

		rule := &ruleNET006{baseRule: baseRule{id: "NET006", severity: SeverityCritical, category: CategoryNetworking}}
		findings, err := rule.Evaluate(context.Background(), cache)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(findings) != 2 {
			t.Fatalf("expected 2 findings, got %d", len(findings))
		}
		for _, f := range findings {
			if f.Severity != SeverityCritical {
				t.Errorf("expected critical severity, got %s", f.Severity)
			}
			if f.Details["host"] != "app.example.com" {
				t.Errorf("expected host app.example.com, got %s", f.Details["host"])
			}
		}
	})

	t.Run("same host different paths no conflict", func(t *testing.T) {
		cache := mockCache(map[string]interface{}{
			"ingresses": []networkingv1.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "app-api", Namespace: "default"},
					Spec: networkingv1.IngressSpec{
						IngressClassName: ptr("nginx"),
						Rules: []networkingv1.IngressRule{{
							Host: "app.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{{Path: "/api", PathType: &prefix}},
							}},
						}},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "app-web", Namespace: "default"},
					Spec: networkingv1.IngressSpec{
						IngressClassName: ptr("nginx"),
						Rules: []networkingv1.IngressRule{{
							Host: "app.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{{Path: "/", PathType: &prefix}},
							}},
						}},
					},
				},
			},
		})

		rule := &ruleNET006{baseRule: baseRule{id: "NET006", severity: SeverityCritical, category: CategoryNetworking}}
		findings, err := rule.Evaluate(context.Background(), cache)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(findings) != 0 {
			t.Fatalf("expected 0 findings for different paths, got %d", len(findings))
		}
	})

	t.Run("same host same path different ingress class still conflicts", func(t *testing.T) {
		cache := mockCache(map[string]interface{}{
			"ingresses": []networkingv1.Ingress{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "influxdb-ai", Namespace: "default"},
					Spec: networkingv1.IngressSpec{
						IngressClassName: ptr("traefik"),
						Rules: []networkingv1.IngressRule{{
							Host: "db.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{{Path: "/", PathType: &prefix}},
							}},
						}},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "influxdb", Namespace: "default"},
					Spec: networkingv1.IngressSpec{
						IngressClassName: ptr("traefik"),
						Rules: []networkingv1.IngressRule{{
							Host: "db.example.com",
							IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
								Paths: []networkingv1.HTTPIngressPath{{Path: "/", PathType: &prefix}},
							}},
						}},
					},
				},
			},
		})

		rule := &ruleNET006{baseRule: baseRule{id: "NET006", severity: SeverityCritical, category: CategoryNetworking}}
		findings, err := rule.Evaluate(context.Background(), cache)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(findings) != 2 {
			t.Fatalf("expected 2 findings, got %d", len(findings))
		}
	})
}

// ---- WRK005-WRK012 Tests ----

func TestWRK005_OOMKilled(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "oom-pod", Namespace: "default"},
				Status: v1.PodStatus{
					Phase: v1.PodRunning,
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name:         "main",
							RestartCount: 3,
							State:        v1.ContainerState{Running: &v1.ContainerStateRunning{}},
							LastTerminationState: v1.ContainerState{
								Terminated: &v1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137},
							},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "healthy-pod", Namespace: "default"},
				Status:     v1.PodStatus{Phase: v1.PodRunning},
			},
		},
	})

	rule := &ruleWRK005{baseRule: baseRule{id: "WRK005", severity: SeverityCritical, category: CategoryWorkloads}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Resource.Name != "oom-pod" {
		t.Errorf("expected oom-pod, got %s", findings[0].Resource.Name)
	}
}

func TestWRK006_PendingPods(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "pending-pod", Namespace: "default"},
				Status:     v1.PodStatus{Phase: v1.PodPending},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "job-pending", Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{{Kind: "Job", Name: "my-job"}},
				},
				Status: v1.PodStatus{Phase: v1.PodPending},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "running-pod", Namespace: "default"},
				Status:     v1.PodStatus{Phase: v1.PodRunning},
			},
		},
	})

	rule := &ruleWRK006{baseRule: baseRule{id: "WRK006", severity: SeverityCritical, category: CategoryWorkloads}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding (job-owned skipped), got %d", len(findings))
	}
	if findings[0].Resource.Name != "pending-pod" {
		t.Errorf("expected pending-pod, got %s", findings[0].Resource.Name)
	}
}

func TestWRK007_ImagePullBackOff(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "pull-fail", Namespace: "default"},
				Status: v1.PodStatus{
					Phase: v1.PodPending,
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name: "main", Image: "registry.example.com/myapp:v99",
							State: v1.ContainerState{Waiting: &v1.ContainerStateWaiting{Reason: "ImagePullBackOff"}},
						},
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "err-pull", Namespace: "default"},
				Status: v1.PodStatus{
					Phase: v1.PodPending,
					ContainerStatuses: []v1.ContainerStatus{
						{
							Name: "sidecar", Image: "nonexistent:latest",
							State: v1.ContainerState{Waiting: &v1.ContainerStateWaiting{Reason: "ErrImagePull"}},
						},
					},
				},
			},
		},
	})

	rule := &ruleWRK007{baseRule: baseRule{id: "WRK007", severity: SeverityCritical, category: CategoryWorkloads}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(findings))
	}
}

func TestWRK008_MissingHealthProbes(t *testing.T) {
	probe := &v1.Probe{ProbeHandler: v1.ProbeHandler{HTTPGet: &v1.HTTPGetAction{Path: "/healthz", Port: intstr.FromInt(8080)}}}
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "no-probes", Namespace: "default"},
				Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "app"}, {Name: "sidecar"}}},
				Status:     v1.PodStatus{Phase: v1.PodRunning},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "with-probes", Namespace: "default"},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "app", LivenessProbe: probe, ReadinessProbe: probe},
				}},
				Status: v1.PodStatus{Phase: v1.PodRunning},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "completed", Namespace: "default"},
				Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "worker"}}},
				Status:     v1.PodStatus{Phase: v1.PodSucceeded},
			},
		},
	})

	rule := &ruleWRK008{baseRule: baseRule{id: "WRK008", severity: SeverityWarning, category: CategoryWorkloads}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Resource.Name != "no-probes" {
		t.Errorf("expected no-probes, got %s", findings[0].Resource.Name)
	}
}

func TestWRK009_StatefulSetReplicaMismatch(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"statefulsets": []appsv1.StatefulSet{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "bad-sts", Namespace: "default"},
				Spec:       appsv1.StatefulSetSpec{Replicas: ptr(int32(3))},
				Status:     appsv1.StatefulSetStatus{ReadyReplicas: 1},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "ok-sts", Namespace: "default"},
				Spec:       appsv1.StatefulSetSpec{Replicas: ptr(int32(2))},
				Status:     appsv1.StatefulSetStatus{ReadyReplicas: 2},
			},
		},
	})

	rule := &ruleWRK009{baseRule: baseRule{id: "WRK009", severity: SeverityWarning, category: CategoryWorkloads}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Resource.Name != "bad-sts" {
		t.Errorf("expected bad-sts, got %s", findings[0].Resource.Name)
	}
}

func TestWRK010_DaemonSetNotFullyScheduled(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"daemonsets": []appsv1.DaemonSet{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "bad-ds", Namespace: "default"},
				Status:     appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 1},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "ok-ds", Namespace: "default"},
				Status:     appsv1.DaemonSetStatus{DesiredNumberScheduled: 5, NumberReady: 5},
			},
		},
	})

	rule := &ruleWRK010{baseRule: baseRule{id: "WRK010", severity: SeverityWarning, category: CategoryWorkloads}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Resource.Name != "bad-ds" {
		t.Errorf("expected bad-ds, got %s", findings[0].Resource.Name)
	}
}

func TestWRK011_EvictedPods(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "evicted-pod", Namespace: "default"},
				Status:     v1.PodStatus{Phase: v1.PodFailed, Reason: "Evicted", Message: "The node was low on resource: memory."},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "failed-pod", Namespace: "default"},
				Status:     v1.PodStatus{Phase: v1.PodFailed, Reason: "Error"},
			},
		},
	})

	rule := &ruleWRK011{baseRule: baseRule{id: "WRK011", severity: SeverityInfo, category: CategoryWorkloads}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Resource.Name != "evicted-pod" {
		t.Errorf("expected evicted-pod, got %s", findings[0].Resource.Name)
	}
}

func TestWRK012_SingleReplicaDeployment(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"deployments": []appsv1.Deployment{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "single-deploy", Namespace: "production"},
				Spec:       appsv1.DeploymentSpec{Replicas: ptr(int32(1))},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "ha-deploy", Namespace: "production"},
				Spec:       appsv1.DeploymentSpec{Replicas: ptr(int32(3))},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "system-deploy", Namespace: "kube-system"},
				Spec:       appsv1.DeploymentSpec{Replicas: ptr(int32(1))},
			},
		},
	})

	rule := &ruleWRK012{baseRule: baseRule{id: "WRK012", severity: SeverityInfo, category: CategoryWorkloads}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding (skip kube-system), got %d", len(findings))
	}
	if findings[0].Resource.Name != "single-deploy" {
		t.Errorf("expected single-deploy, got %s", findings[0].Resource.Name)
	}
}

// ---- SEC003-SEC007 Tests ----

func TestSEC003_PrivilegedContainers(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "priv-pod", Namespace: "default"},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "privileged", SecurityContext: &v1.SecurityContext{Privileged: ptr(true)}},
					{Name: "normal"},
				}},
				Status: v1.PodStatus{Phase: v1.PodRunning},
			},
		},
	})

	rule := &ruleSEC003{baseRule: baseRule{id: "SEC003", severity: SeverityWarning, category: CategorySecurity}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestSEC004_HostNamespaces(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "host-pod", Namespace: "default"},
				Spec:       v1.PodSpec{HostNetwork: true, HostPID: true, Containers: []v1.Container{{Name: "app"}}},
				Status:     v1.PodStatus{Phase: v1.PodRunning},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "normal-pod", Namespace: "default"},
				Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "app"}}},
				Status:     v1.PodStatus{Phase: v1.PodRunning},
			},
		},
	})

	rule := &ruleSEC004{baseRule: baseRule{id: "SEC004", severity: SeverityWarning, category: CategorySecurity}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Resource.Name != "host-pod" {
		t.Errorf("expected host-pod, got %s", findings[0].Resource.Name)
	}
}

func TestSEC005_WritableRootFilesystem(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "writable-pod", Namespace: "default"},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "readonly", SecurityContext: &v1.SecurityContext{ReadOnlyRootFilesystem: ptr(true)}},
					{Name: "writable"},
				}},
				Status: v1.PodStatus{Phase: v1.PodRunning},
			},
		},
	})

	rule := &ruleSEC005{baseRule: baseRule{id: "SEC005", severity: SeverityInfo, category: CategorySecurity}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestSEC006_PrivilegeEscalation(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "escalation-pod", Namespace: "default"},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{Name: "safe", SecurityContext: &v1.SecurityContext{AllowPrivilegeEscalation: ptr(false)}},
					{Name: "unsafe"}, // nil securityContext — allows escalation
				}},
				Status: v1.PodStatus{Phase: v1.PodRunning},
			},
		},
	})

	rule := &ruleSEC006{baseRule: baseRule{id: "SEC006", severity: SeverityWarning, category: CategorySecurity}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

func TestSEC007_CapabilityAdditions(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "cap-pod", Namespace: "default"},
				Spec: v1.PodSpec{Containers: []v1.Container{
					{
						Name: "admin",
						SecurityContext: &v1.SecurityContext{
							Capabilities: &v1.Capabilities{Add: []v1.Capability{"SYS_ADMIN", "NET_ADMIN"}},
						},
					},
					{Name: "normal"},
				}},
				Status: v1.PodStatus{Phase: v1.PodRunning},
			},
		},
	})

	rule := &ruleSEC007{baseRule: baseRule{id: "SEC007", severity: SeverityWarning, category: CategorySecurity}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
}

// ---- NET007-NET009 Tests ----

func TestNET007_ExternalNameDangling(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"services": []v1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "ext-empty", Namespace: "default"},
				Spec:       v1.ServiceSpec{Type: v1.ServiceTypeExternalName, ExternalName: ""},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "ext-ok", Namespace: "default"},
				Spec:       v1.ServiceSpec{Type: v1.ServiceTypeExternalName, ExternalName: "db.example.com"},
			},
		},
	})

	rule := &ruleNET007{baseRule: baseRule{id: "NET007", severity: SeverityWarning, category: CategoryNetworking}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Resource.Name != "ext-empty" {
		t.Errorf("expected ext-empty, got %s", findings[0].Resource.Name)
	}
}

func TestNET008_LoadBalancerPending(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"services": []v1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "lb-pending", Namespace: "default"},
				Spec:       v1.ServiceSpec{Type: v1.ServiceTypeLoadBalancer},
				Status:     v1.ServiceStatus{},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "lb-ok", Namespace: "default"},
				Spec:       v1.ServiceSpec{Type: v1.ServiceTypeLoadBalancer},
				Status: v1.ServiceStatus{
					LoadBalancer: v1.LoadBalancerStatus{
						Ingress: []v1.LoadBalancerIngress{{IP: "1.2.3.4"}},
					},
				},
			},
		},
	})

	rule := &ruleNET008{baseRule: baseRule{id: "NET008", severity: SeverityWarning, category: CategoryNetworking}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Resource.Name != "lb-pending" {
		t.Errorf("expected lb-pending, got %s", findings[0].Resource.Name)
	}
}

func TestNET009_NodePortService(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"services": []v1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "np-svc", Namespace: "default"},
				Spec:       v1.ServiceSpec{Type: v1.ServiceTypeNodePort},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "np-system", Namespace: "kube-system"},
				Spec:       v1.ServiceSpec{Type: v1.ServiceTypeNodePort},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "cluster-svc", Namespace: "default"},
				Spec:       v1.ServiceSpec{Type: v1.ServiceTypeClusterIP},
			},
		},
	})

	rule := &ruleNET009{baseRule: baseRule{id: "NET009", severity: SeverityInfo, category: CategoryNetworking}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding (skip kube-system), got %d", len(findings))
	}
	if findings[0].Resource.Name != "np-svc" {
		t.Errorf("expected np-svc, got %s", findings[0].Resource.Name)
	}
}

// ---- CFG003-CFG004 Tests ----

func TestCFG003_UnreferencedSecret(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"secrets": []v1.Secret{
			{ObjectMeta: metav1.ObjectMeta{Name: "unused-secret", Namespace: "default"}, Type: v1.SecretTypeOpaque},
			{ObjectMeta: metav1.ObjectMeta{Name: "used-secret", Namespace: "default"}, Type: v1.SecretTypeOpaque},
			{ObjectMeta: metav1.ObjectMeta{Name: "sa-token", Namespace: "default"}, Type: v1.SecretTypeServiceAccountToken},
			{ObjectMeta: metav1.ObjectMeta{Name: "helm-release", Namespace: "default"}, Type: "helm.sh/release.v1"},
		},
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "my-pod", Namespace: "default"},
				Spec: v1.PodSpec{
					Volumes: []v1.Volume{
						{Name: "sec-vol", VolumeSource: v1.VolumeSource{Secret: &v1.SecretVolumeSource{SecretName: "used-secret"}}},
					},
					Containers: []v1.Container{{Name: "main"}},
				},
				Status: v1.PodStatus{Phase: v1.PodRunning},
			},
		},
		"serviceaccounts": []v1.ServiceAccount{},
	})

	rule := &ruleCFG003{baseRule: baseRule{id: "CFG003", severity: SeverityInfo, category: CategoryConfig}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding (unused-secret only), got %d", len(findings))
	}
	if findings[0].Resource.Name != "unused-secret" {
		t.Errorf("expected unused-secret, got %s", findings[0].Resource.Name)
	}
}

func TestCFG004_NoResourceRequests(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "no-requests", Namespace: "default"},
				Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "main"}, {Name: "sidecar"}}},
				Status:     v1.PodStatus{Phase: v1.PodRunning},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "completed", Namespace: "default"},
				Spec:       v1.PodSpec{Containers: []v1.Container{{Name: "main"}}},
				Status:     v1.PodStatus{Phase: v1.PodSucceeded},
			},
		},
	})

	rule := &ruleCFG004{baseRule: baseRule{id: "CFG004", severity: SeverityWarning, category: CategoryConfig}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Resource.Name != "no-requests" {
		t.Errorf("expected no-requests, got %s", findings[0].Resource.Name)
	}
}

// ---- STR003 Test ----

func TestSTR003_PVCPending(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pvcs": []v1.PersistentVolumeClaim{
			{
				ObjectMeta: metav1.ObjectMeta{Name: "pending-pvc", Namespace: "default"},
				Status:     v1.PersistentVolumeClaimStatus{Phase: v1.ClaimPending},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "bound-pvc", Namespace: "default"},
				Status:     v1.PersistentVolumeClaimStatus{Phase: v1.ClaimBound},
			},
		},
	})

	rule := &ruleSTR003{baseRule: baseRule{id: "STR003", severity: SeverityWarning, category: CategoryStorage}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Resource.Name != "pending-pvc" {
		t.Errorf("expected pending-pvc, got %s", findings[0].Resource.Name)
	}
}

// ---- Deprecation Rules Tests ----

func TestDEP001_DeprecatedEndpoints(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"endpoints": []v1.Endpoints{
			{ObjectMeta: metav1.ObjectMeta{Name: "my-svc", Namespace: "default"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "kube-dns", Namespace: "kube-system"}},
		},
	})

	rule := &ruleDEP001{baseRule: baseRule{id: "DEP001", severity: SeverityWarning, category: CategoryDeprecation}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding (system ns skipped), got %d", len(findings))
	}
}

func TestDEP002_DeprecatedIngressClassAnnotation(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"ingresses": []networkingv1.Ingress{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "old-ing", Namespace: "default",
					Annotations: map[string]string{"kubernetes.io/ingress.class": "nginx"},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "new-ing", Namespace: "default"},
				Spec:       networkingv1.IngressSpec{IngressClassName: ptr("nginx")},
			},
		},
	})

	rule := &ruleDEP002{baseRule: baseRule{id: "DEP002", severity: SeverityWarning, category: CategoryDeprecation}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Resource.Name != "old-ing" {
		t.Errorf("expected old-ing, got %s", findings[0].Resource.Name)
	}
}

func TestDEP003_DeprecatedNodeTopologyLabels(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"nodes": []v1.Node{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "old-node",
					Labels: map[string]string{"failure-domain.beta.kubernetes.io/zone": "us-east-1a"},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "new-node",
					Labels: map[string]string{"topology.kubernetes.io/zone": "us-east-1b"},
				},
			},
		},
	})

	rule := &ruleDEP003{baseRule: baseRule{id: "DEP003", severity: SeverityWarning, category: CategoryDeprecation}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Resource.Name != "old-node" {
		t.Errorf("expected old-node, got %s", findings[0].Resource.Name)
	}
}

func TestDEP004_DeprecatedSeccompAnnotations(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "seccomp-pod", Namespace: "default",
					Annotations: map[string]string{"seccomp.security.alpha.kubernetes.io/pod": "runtime/default"},
				},
				Status: v1.PodStatus{Phase: v1.PodRunning},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "completed-pod", Namespace: "default",
					Annotations: map[string]string{"seccomp.security.alpha.kubernetes.io/pod": "runtime/default"},
				},
				Status: v1.PodStatus{Phase: v1.PodSucceeded},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "clean-pod", Namespace: "default"},
				Status:     v1.PodStatus{Phase: v1.PodRunning},
			},
		},
	})

	rule := &ruleDEP004{baseRule: baseRule{id: "DEP004", severity: SeverityWarning, category: CategoryDeprecation}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding (completed skipped), got %d", len(findings))
	}
	if findings[0].Resource.Name != "seccomp-pod" {
		t.Errorf("expected seccomp-pod, got %s", findings[0].Resource.Name)
	}
}

func TestDEP005_DeprecatedAppArmorAnnotations(t *testing.T) {
	cache := mockCache(map[string]interface{}{
		"pods": []v1.Pod{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "apparmor-pod", Namespace: "default",
					Annotations: map[string]string{"container.apparmor.security.beta.kubernetes.io/main": "runtime/default"},
				},
				Status: v1.PodStatus{Phase: v1.PodRunning},
			},
			{
				ObjectMeta: metav1.ObjectMeta{Name: "clean-pod", Namespace: "default"},
				Status:     v1.PodStatus{Phase: v1.PodRunning},
			},
		},
	})

	rule := &ruleDEP005{baseRule: baseRule{id: "DEP005", severity: SeverityWarning, category: CategoryDeprecation}}
	findings, err := rule.Evaluate(context.Background(), cache)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Resource.Name != "apparmor-pod" {
		t.Errorf("expected apparmor-pod, got %s", findings[0].Resource.Name)
	}
}
