package acceleratoracceptance

import "testing"

func TestAcceleratorClusterScopeReleasedIsOwnershipExactAndClosed(t *testing.T) {
	for _, test := range []struct {
		name      string
		encoded   string
		namespace string
		want      bool
	}{
		{name: "empty", encoded: `{"apiVersion":"v1","kind":"List","items":[]}`, namespace: "owned", want: true},
		{name: "foreign retained", encoded: `{"apiVersion":"v1","kind":"List","items":[{"metadata":{"annotations":{"meta.helm.sh/release-namespace":"foreign"}}}]}`, namespace: "owned", want: true},
		{name: "owned retained", encoded: `{"apiVersion":"v1","kind":"List","items":[{"metadata":{"annotations":{"meta.helm.sh/release-namespace":"owned"}}}]}`, namespace: "owned"},
		{name: "missing items", encoded: `{"apiVersion":"v1","kind":"List"}`, namespace: "owned"},
		{name: "wrong kind", encoded: `{"apiVersion":"v1","kind":"ClusterRoleList","items":[]}`, namespace: "owned"},
		{name: "malformed", encoded: `{`, namespace: "owned"},
		{name: "empty namespace", encoded: `{"apiVersion":"v1","kind":"List","items":[]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := acceleratorClusterScopeReleased([]byte(test.encoded), test.namespace); got != test.want {
				t.Fatalf("released=%v want=%v", got, test.want)
			}
		})
	}
}
