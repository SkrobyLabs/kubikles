package store

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Key identifies one shard. Type uses the strings the watcher manager already
// uses ("pods", "crd:group/version/resource"). An empty Namespace means
// cluster wide.
type Key struct {
	Context   string
	Type      string
	Namespace string
}

func (k Key) String() string {
	return k.Context + "|" + k.Type + "|" + k.Namespace
}

// ClusterWide returns the key of the cluster-wide shard for the same type.
func (k Key) ClusterWide() Key {
	k.Namespace = ""
	return k
}

const crdPrefix = "crd:"

// CRDType builds the Type string for a custom resource.
func CRDType(group, version, resource string) string {
	return fmt.Sprintf("%s%s/%s/%s", crdPrefix, group, version, resource)
}

// ParseCRDType is the inverse of CRDType. The group may be empty (core-style
// resources served through the dynamic client).
func ParseCRDType(t string) (schema.GroupVersionResource, bool) {
	if !strings.HasPrefix(t, crdPrefix) {
		return schema.GroupVersionResource{}, false
	}
	parts := strings.Split(strings.TrimPrefix(t, crdPrefix), "/")
	if len(parts) != 3 || parts[1] == "" || parts[2] == "" {
		return schema.GroupVersionResource{}, false
	}
	return schema.GroupVersionResource{Group: parts[0], Version: parts[1], Resource: parts[2]}, true
}
