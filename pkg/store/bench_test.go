package store

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
)

// memSource generates pods page by page, so no copy of the full list stays
// reachable and a heap measurement reflects only what the shard keeps.
type memSource struct {
	n        int
	pageSize int
}

func (s *memSource) Mode() Mode { return ModeFull }
func (s *memSource) List(_ context.Context, _ ListOpts, onPage func(Page)) (string, error) {
	loaded := 0
	for i := 0; i < s.n; i += s.pageSize {
		end := min(i+s.pageSize, s.n)
		page := &v1.PodList{Items: make([]v1.Pod, 0, end-i)}
		for j := i; j < end; j++ {
			page.Items = append(page.Items, realisticPod(j))
		}
		items, err := extractItems(page)
		if err != nil {
			return "", err
		}
		loaded += len(items)
		onPage(Page{Items: items, Loaded: loaded})
	}
	return "1", nil
}
func (s *memSource) Watch(context.Context, string) (watch.Interface, error) {
	return watch.NewFake(), nil
}

// realisticPod approximates a mid-sized workload pod: two containers, env,
// probes, resources, labels, annotations and a populated status.
func realisticPod(i int) v1.Pod {
	ns := fmt.Sprintf("ns-%d", i%40)
	name := fmt.Sprintf("workload-%d-7d9f8c6b5d-x%04d", i/5, i)
	c := func(n string) v1.Container {
		return v1.Container{
			Name: n, Image: "registry.example.com/team/" + n + ":v1.2.3",
			Args: []string{"--config=/etc/app/config.yaml", "--log-level=info", "--port=8080"},
			Env: []v1.EnvVar{{Name: "POD_NAME", ValueFrom: &v1.EnvVarSource{FieldRef: &v1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
				{Name: "LOG_FORMAT", Value: "json"}, {Name: "REGION", Value: "eu-west-1"}},
			Ports: []v1.ContainerPort{{Name: "http", ContainerPort: 8080, Protocol: v1.ProtocolTCP}},
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{v1.ResourceCPU: resource.MustParse("100m"), v1.ResourceMemory: resource.MustParse("128Mi")},
				Limits:   v1.ResourceList{v1.ResourceMemory: resource.MustParse("256Mi")},
			},
			VolumeMounts: []v1.VolumeMount{{Name: "config", MountPath: "/etc/app"}, {Name: "kube-api-access", MountPath: "/var/run/secrets"}},
		}
	}
	return v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: ns, UID: types.UID(fmt.Sprintf("00000000-0000-4000-8000-%012d", i)), ResourceVersion: fmt.Sprint(1000 + i),
			Labels:          map[string]string{"app": fmt.Sprintf("workload-%d", i/5), "pod-template-hash": "7d9f8c6b5d", "team": "platform"},
			Annotations:     map[string]string{"prometheus.io/scrape": "true", "kubectl.kubernetes.io/restartedAt": "2026-01-02T03:04:05Z"},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: fmt.Sprintf("workload-%d-7d9f8c6b5d", i/5), UID: "rs-uid"}},
			ManagedFields:   []metav1.ManagedFieldsEntry{{Manager: "kube-controller-manager"}, {Manager: "kubelet"}},
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{c("app"), c("sidecar")}, NodeName: fmt.Sprintf("node-%d", i%200),
			ServiceAccountName: "default", Volumes: []v1.Volume{{Name: "config"}, {Name: "kube-api-access"}},
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning, HostIP: "10.0.0.1", PodIP: fmt.Sprintf("10.1.%d.%d", i/250%250, i%250),
			ContainerStatuses: []v1.ContainerStatus{
				{Name: "app", Ready: true, RestartCount: 1, Image: "registry.example.com/team/app:v1.2.3", ContainerID: "containerd://abcdef0123456789abcdef0123456789"},
				{Name: "sidecar", Ready: true, Image: "registry.example.com/team/sidecar:v1.2.3", ContainerID: "containerd://0123456789abcdef0123456789abcdef"},
			},
			Conditions: []v1.PodCondition{{Type: v1.PodReady, Status: v1.ConditionTrue}, {Type: v1.ContainersReady, Status: v1.ConditionTrue}},
		},
	}
}

func newBenchShard(src Source) *Shard {
	cfg := shardConfig{
		key: Key{Context: "bench", Type: "pods"}, clock: realClock{},
		pageSize: func() int64 { return 1000 }, pollEvery: func() time.Duration { return 0 },
		baseDelay: time.Second, maxDelay: time.Minute,
	}
	return newShard(cfg, src, ModeFull, ModeFull, "")
}

// BenchmarkIngest50kPods measures ingesting a 50,000 pod LIST into a shard
// (conversion to entries, namespace index, change publication) and reports the
// resident heap per entry next to the sampled encoded size, which is how
// heapFactor and the default byte budget were chosen.
func BenchmarkIngest50kPods(b *testing.B) {
	const n = 50000
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src := &memSource{n: n, pageSize: 1000}
		s := newBenchShard(src)
		if _, err := s.list(context.Background()); err != nil {
			b.Fatal(err)
		}
		if s.State().Entries != n {
			b.Fatalf("entries = %d", s.State().Entries)
		}
	}
}

// TestIngestHeapPerEntry records the memory behind heapFactor. It is a test
// rather than a benchmark so the ratio is asserted, not just printed.
func TestIngestHeapPerEntry(t *testing.T) {
	if testing.Short() {
		t.Skip("short")
	}
	const n = 50000
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	s := newBenchShard(&memSource{n: n, pageSize: 1000})
	if _, err := s.list(context.Background()); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	heap := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	est := s.estimatedBytes()
	perEntry := heap / n
	avgEncoded := s.sampleBytes.Load() / s.sampleCount.Load()
	t.Logf("50k pods: heap %.1f MiB (%d B/entry), encoded %d B/entry, estimate %.1f MiB, heap/encoded = %.2f",
		float64(heap)/(1<<20), perEntry, avgEncoded, float64(est)/(1<<20), float64(heap)/float64(avgEncoded*n))
	if est < heap/2 || est > heap*2 {
		t.Fatalf("estimate %d is not within 2x of measured heap %d; recalibrate heapFactor", est, heap)
	}
	runtime.KeepAlive(s)
}
