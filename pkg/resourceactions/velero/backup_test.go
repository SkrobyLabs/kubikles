package velero

import (
	"context"
	"testing"

	"kubikles/pkg/resourceactions"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestRepeatBackupRequiresTerminalOriginal(t *testing.T) {
	for _, phase := range []string{"InProgress", "Completed", "Failed"} {
		t.Run(phase, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "velero.io/v1", "kind": "Backup", "metadata": map[string]interface{}{"name": "backup", "namespace": "velero", "uid": "uid", "resourceVersion": "1"}, "spec": map[string]interface{}{"storageLocation": "store"}, "status": map[string]interface{}{"phase": phase}}}
			c := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)
			ref := resourceactions.ResourceRef{Context: "dev", Group: "velero.io", Version: "v1", Resource: "backups", Namespace: "velero", Name: "backup", UID: "uid"}
			r := resourceactions.New(BackupProvider{})
			plan, err := r.Prepare(context.Background(), c, ref, "velero.backup.repeat", "backup")
			if phase == "InProgress" {
				if err == nil {
					t.Fatal("accepted active backup")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			results, err := r.Execute(context.Background(), c, plan)
			if err != nil || results[0].Status != "requested" {
				t.Fatalf("%+v %v", results, err)
			}
		})
	}
}
