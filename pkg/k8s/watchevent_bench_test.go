package k8s

import (
	"bytes"
	"encoding/json"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	jsonserializer "k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/runtime/serializer/protobuf"
	"k8s.io/client-go/kubernetes/scheme"
)

// benchWireEvent mirrors the shape the watch loop sends to the frontend
// (main.ResourceEvent) so the encode benchmarks include the same outer cost.
type benchWireEvent struct {
	Type         string                 `json:"type"`
	ResourceType string                 `json:"resourceType"`
	Namespace    string                 `json:"namespace"`
	Resource     map[string]interface{} `json:"resource"`
}

// BenchmarkWatchEventEncodeCurrent measures what the watch loop does per
// event today: typed object -> map -> JSON for the frontend.
func BenchmarkWatchEventEncodeCurrent(b *testing.B) {
	pod := newRichPod(1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m, err := RuntimeObjectToMap(pod)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := json.Marshal(benchWireEvent{Type: "MODIFIED", ResourceType: "pods", Namespace: "production", Resource: m}); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWatchEventEncodeRuntimeObjectToMap isolates the conversion step.
func BenchmarkWatchEventEncodeRuntimeObjectToMap(b *testing.B) {
	pod := newRichPod(1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := RuntimeObjectToMap(pod); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWatchEventEncodeMarshalMap isolates the JSON step on an existing map.
func BenchmarkWatchEventEncodeMarshalMap(b *testing.B) {
	m, err := RuntimeObjectToMap(newRichPod(1))
	if err != nil {
		b.Fatal(err)
	}
	ev := benchWireEvent{Type: "MODIFIED", ResourceType: "pods", Namespace: "production", Resource: m}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := json.Marshal(ev); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkWatchEventEncodeDirectJSON is the floor for a typed-object -> JSON
// implementation that skips the intermediate map: marshal the typed object
// once and splice it into the envelope as json.RawMessage.
func BenchmarkWatchEventEncodeDirectJSON(b *testing.B) {
	pod := newRichPod(1)
	type rawEvent struct {
		Type         string          `json:"type"`
		ResourceType string          `json:"resourceType"`
		Namespace    string          `json:"namespace"`
		Resource     json.RawMessage `json:"resource"`
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		raw, err := json.Marshal(pod)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := json.Marshal(rawEvent{Type: "MODIFIED", ResourceType: "pods", Namespace: "production", Resource: raw}); err != nil {
			b.Fatal(err)
		}
	}
}

// benchPodList builds a realistic list of n pods.
func benchPodList(n int) *corev1.PodList {
	list := &corev1.PodList{ListMeta: metav1.ListMeta{ResourceVersion: "123456"}}
	list.Items = make([]corev1.Pod, n)
	for i := 0; i < n; i++ {
		list.Items[i] = *newRichPod(i)
	}
	return list
}

func encodePodListProtobuf(b *testing.B, list *corev1.PodList) []byte {
	b.Helper()
	list = list.DeepCopy()
	list.APIVersion, list.Kind = "v1", "PodList"
	var buf bytes.Buffer
	if err := protobuf.NewSerializer(scheme.Scheme, scheme.Scheme).Encode(list, &buf); err != nil {
		b.Fatal(err)
	}
	return buf.Bytes()
}

func encodePodListJSON(b *testing.B, list *corev1.PodList) []byte {
	b.Helper()
	list = list.DeepCopy()
	list.APIVersion, list.Kind = "v1", "PodList"
	var buf bytes.Buffer
	ser := jsonserializer.NewSerializerWithOptions(jsonserializer.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, jsonserializer.SerializerOptions{})
	if err := ser.Encode(list, &buf); err != nil {
		b.Fatal(err)
	}
	return buf.Bytes()
}

func decodeInto(b *testing.B, d runtime.Decoder, data []byte) *corev1.PodList {
	b.Helper()
	obj, _, err := d.Decode(data, nil, &corev1.PodList{})
	if err != nil {
		b.Fatal(err)
	}
	list, ok := obj.(*corev1.PodList)
	if !ok {
		b.Fatalf("decoded %T", obj)
	}
	return list
}

// BenchmarkDecodePodList1000 compares decoding a 1000-pod PodList response
// from JSON (the default) and protobuf (application/vnd.kubernetes.protobuf).
func BenchmarkDecodePodList1000(b *testing.B) {
	list := benchPodList(1000)
	jsonData := encodePodListJSON(b, list)
	pbData := encodePodListProtobuf(b, list)

	jsonDec := jsonserializer.NewSerializerWithOptions(jsonserializer.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, jsonserializer.SerializerOptions{})
	pbDec := protobuf.NewSerializer(scheme.Scheme, scheme.Scheme)

	// Sanity: both encodings decode to the same list.
	if got, want := len(decodeInto(b, jsonDec, jsonData).Items), 1000; got != want {
		b.Fatalf("json items = %d", got)
	}
	if got, want := len(decodeInto(b, pbDec, pbData).Items), 1000; got != want {
		b.Fatalf("protobuf items = %d", got)
	}

	b.Run("JSON", func(b *testing.B) {
		b.SetBytes(int64(len(jsonData)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			decodeInto(b, jsonDec, jsonData)
		}
	})
	b.Run("Protobuf", func(b *testing.B) {
		b.SetBytes(int64(len(pbData)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			decodeInto(b, pbDec, pbData)
		}
	})
	b.Run("EncodedSizes", func(b *testing.B) {
		b.ReportMetric(float64(len(jsonData)), "json-bytes")
		b.ReportMetric(float64(len(pbData)), "protobuf-bytes")
		for i := 0; i < b.N; i++ {
		}
	})
}

// TestPodListProtobufRoundTripMatchesJSON guards the benchmark premise: the
// two wire formats must carry the same data, otherwise the comparison is moot.
func TestPodListProtobufRoundTripMatchesJSON(t *testing.T) {
	list := benchPodList(5)
	var pbBuf, jsBuf bytes.Buffer
	l := list.DeepCopy()
	l.APIVersion, l.Kind = "v1", "PodList"
	if err := protobuf.NewSerializer(scheme.Scheme, scheme.Scheme).Encode(l, &pbBuf); err != nil {
		t.Fatal(err)
	}
	if err := jsonserializer.NewSerializerWithOptions(jsonserializer.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, jsonserializer.SerializerOptions{}).Encode(l, &jsBuf); err != nil {
		t.Fatal(err)
	}
	pbObj, _, err := protobuf.NewSerializer(scheme.Scheme, scheme.Scheme).Decode(pbBuf.Bytes(), nil, &corev1.PodList{})
	if err != nil {
		t.Fatal(err)
	}
	jsObj, _, err := jsonserializer.NewSerializerWithOptions(jsonserializer.DefaultMetaFactory, scheme.Scheme, scheme.Scheme, jsonserializer.SerializerOptions{}).Decode(jsBuf.Bytes(), nil, &corev1.PodList{})
	if err != nil {
		t.Fatal(err)
	}
	for i := range list.Items {
		a := canonicalJSON(t, pbObj.(*corev1.PodList).Items[i])
		b := canonicalJSON(t, jsObj.(*corev1.PodList).Items[i])
		if a != b {
			t.Fatalf("pod %d differs between protobuf and JSON decode\n%s\n%s", i, a, b)
		}
	}
}

// BenchmarkWatchEventEncodeEncodeWatchObject measures the production path:
// strip managedFields, marshal the typed object once, splice it into the
// envelope as json.RawMessage. managedFields are restored each iteration so
// the strip cost is included.
func BenchmarkWatchEventEncodeEncodeWatchObject(b *testing.B) {
	pod := newRichPod(1)
	managed := pod.ManagedFields
	type rawEvent struct {
		Type         string          `json:"type"`
		ResourceType string          `json:"resourceType"`
		Namespace    string          `json:"namespace"`
		Resource     json.RawMessage `json:"resource"`
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pod.ManagedFields = managed
		info, raw, err := EncodeWatchObject(pod)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := json.Marshal(rawEvent{Type: "MODIFIED", ResourceType: "pods", Namespace: info.Namespace, Resource: raw}); err != nil {
			b.Fatal(err)
		}
	}
}
