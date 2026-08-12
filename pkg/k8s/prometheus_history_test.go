package k8s

import (
	"strings"
	"testing"
	"time"
)

func TestNodeMemoryDiagnosticQueriesUseBucketSemantics(t *testing.T) {
	step := 5 * time.Minute
	available := nodeMemoryBucketQuery("min", `node_memory_MemAvailable_bytes{node="node-a"}`, step)
	workingSet := nodeMemoryBucketQuery("max", `container_memory_working_set_bytes{node="node-a", id="/"}`, step)
	contributors := nodeMemoryContributorQuery("node-a", step)

	if !strings.Contains(available, "min_over_time") || !strings.Contains(available, "[300s:]") {
		t.Fatalf("availability query must use a minimum step bucket: %s", available)
	}
	if !strings.Contains(workingSet, "max_over_time") || !strings.Contains(workingSet, "[300s:]") {
		t.Fatalf("working set query must use a maximum step bucket: %s", workingSet)
	}
	if !strings.Contains(contributors, `topk(5, max_over_time`) || !strings.Contains(contributors, `node="node-a"`) {
		t.Fatalf("contributor query must preserve node identity and bounded maximum buckets: %s", contributors)
	}
}

func TestParseNodeMemoryContributorsRanksAndBounds(t *testing.T) {
	series := []struct {
		Metric map[string]string `json:"metric"`
		Value  []interface{}     `json:"value"`
		Values [][]interface{}   `json:"values"`
	}{
		{Metric: map[string]string{"namespace": "z", "pod": "equal"}, Values: [][]interface{}{{float64(1), "10"}}},
		{Metric: map[string]string{"namespace": "a", "pod": "equal"}, Values: [][]interface{}{{float64(1), "10"}}},
		{Metric: map[string]string{"namespace": "ns", "pod": "peak"}, Values: [][]interface{}{{float64(1), "20"}, {float64(2), "invalid"}}},
		{Metric: map[string]string{"namespace": "ns", "pod": "bad"}, Values: [][]interface{}{{"bad", "1"}}},
		{Metric: map[string]string{"namespace": "", "pod": "missing"}, Values: [][]interface{}{{float64(1), "100"}}},
		{Metric: map[string]string{"namespace": "ns", "pod": "four"}, Values: [][]interface{}{{float64(1), "4"}}},
		{Metric: map[string]string{"namespace": "ns", "pod": "five"}, Values: [][]interface{}{{float64(1), "5"}}},
		{Metric: map[string]string{"namespace": "ns", "pod": "six"}, Values: [][]interface{}{{float64(1), "6"}}},
	}

	contributors := parseNodeMemoryContributors(series)
	if len(contributors) != 5 {
		t.Fatalf("got %d contributors, want 5", len(contributors))
	}
	if contributors[0].Pod != "peak" || contributors[1].Namespace != "a" || contributors[1].Pod != "equal" {
		t.Fatalf("contributors were not ranked by peak then stable identity: %#v", contributors)
	}
	if len(contributors[0].WorkingSet) != 1 || contributors[0].WorkingSet[0].Timestamp != 1000 {
		t.Fatalf("valid points were not retained: %#v", contributors[0].WorkingSet)
	}
}
