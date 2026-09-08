package velero

import (
	"testing"

	"kubikles/pkg/resourceactions"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestTransferCancellationCapabilities(t *testing.T) {
	for _, tc := range []struct {
		phase, mover           string
		wantPending, wantError bool
	}{{"InProgress", "velero", false, false}, {"Canceling", "", true, false}, {"Canceled", "", true, false}, {"Completed", "", false, true}, {"Failed", "", false, true}, {"InProgress", "external", false, true}} {
		t.Run(tc.phase+tc.mover, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: map[string]interface{}{"spec": map[string]interface{}{"datamover": tc.mover}, "status": map[string]interface{}{"phase": tc.phase}}}
			mutation, err := (TransferProvider{}).Mutation(resourceactions.Plan{}, resourceactions.Target{}, obj)
			if (err != nil) != tc.wantError || mutation.Pending != tc.wantPending {
				t.Fatalf("%+v %v", mutation, err)
			}
			if !tc.wantError && !tc.wantPending && (len(mutation.Operations) != 1 || mutation.Operations[0]["path"] != "/spec/cancel" || mutation.Operations[0]["value"] != true) {
				t.Fatal(mutation)
			}
		})
	}
}
