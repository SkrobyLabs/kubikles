// Package secretfixtures contains deterministic, test-only Secret fixtures.
package secretfixtures

import (
	"bytes"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	NamespaceA = "baseline-a"
	NamespaceB = "baseline-b"
	HelmType   = "helm.sh/release.v1"
)

var timestamp = metav1.NewTime(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))

// Secrets returns fresh copies of the seven baseline fixtures. Values are generated,
// rather than stored as literals, so this package never writes secret data to disk.
func Secrets() []*corev1.Secret {
	makeValue := func(n int, seed byte) []byte { return bytes.Repeat([]byte{seed}, n) }
	return []*corev1.Secret{
		secret("empty", NamespaceA, "uid-empty", corev1.SecretTypeOpaque, nil),
		secret("text", NamespaceA, "uid-text", corev1.SecretTypeOpaque, map[string][]byte{"a": makeValue(32, 'a'), "b": makeValue(256, 'b')}),
		secret("binary", NamespaceA, "uid-binary", corev1.SecretTypeOpaque, map[string][]byte{"bin": append([]byte{0xff, 0xfe}, makeValue(1022, 0x80)...)}),
		secret("helm", NamespaceA, "uid-helm", corev1.SecretType(HelmType), map[string][]byte{"release": makeValue(4096, 'h')}),
		secret("churn", NamespaceA, "uid-churn", corev1.SecretTypeOpaque, map[string][]byte{"state": makeValue(16, 'c')}),
		secret("yaml", NamespaceB, "uid-yaml", corev1.SecretTypeOpaque, map[string][]byte{"text": makeValue(64, 'y'), "bin": append([]byte{0xff}, makeValue(127, 0x81)...)}),
		secret("large", NamespaceB, "uid-large", corev1.SecretTypeOpaque, map[string][]byte{"blob": makeValue(524288, 'l')}),
	}
}

func secret(name, namespace, uid string, typ corev1.SecretType, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, UID: types.UID(uid), CreationTimestamp: timestamp}, Type: typ, Data: data}
}

// ChurnEvents returns exactly ADDED, 32 MODIFIED, and DELETED for one UID.
func ChurnEvents() []corev1.Secret {
	base := Secrets()[4]
	events := make([]corev1.Secret, 0, 34)
	events = append(events, *base.DeepCopy())
	for i := 1; i <= 32; i++ {
		next := base.DeepCopy()
		next.ResourceVersion = fmt.Sprintf("%d", i)
		events = append(events, *next)
	}
	events = append(events, *base.DeepCopy())
	return events
}
