package main

import (
	"testing"

	"kubikles/pkg/rows"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestReplacedObjectIsNotTheSelectedOne(t *testing.T) {
	pod := func(uid string) *corev1.Pod {
		return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ns", Name: "web-0", UID: types.UID(uid)}}
	}
	cases := []struct {
		name string
		ref  rows.ObjectRef
		obj  *corev1.Pod
		want bool
	}{
		{"same object", rows.ObjectRef{Name: "web-0", UID: "aaa"}, pod("aaa"), false},
		{"recreated under the same name", rows.ObjectRef{Name: "web-0", UID: "aaa"}, pod("bbb"), true},
		{"lookup by name only", rows.ObjectRef{Name: "web-0"}, pod("bbb"), false},
		{"row id that is a namespace/name key, not a uid", rows.ObjectRef{Name: "web-0", UID: "ns/web-0"}, pod("bbb"), false},
		{"object without a uid", rows.ObjectRef{Name: "web-0", UID: "aaa"}, pod(""), false},
	}
	for _, c := range cases {
		if got := replacedObject(c.ref, c.obj); got != c.want {
			t.Errorf("%s: replacedObject = %v, want %v", c.name, got, c.want)
		}
	}
}
