package acceleratorsecret

import (
	"encoding/json"
	"strings"
	"testing"

	"kubikles/pkg/k8s"
)

func Test20BContractAliasCompatibility(t *testing.T) {
	id := SecretWatchSpecIDFor("team", true)
	if id != "yPYiyr7uVHfeARtua3hig9lQX_Baf2ZXkrLsI4WOvlk" || !ValidSecretWatchSpecID(id) {
		t.Fatalf("spec ID=%q", id)
	}
	rich := k8s.SecretListItem{Type: "Opaque", DataKeys: 2}
	rich.Metadata.Labels = map[string]string{"labels-marker": "value-marker"}
	rich.Metadata.Annotations = map[string]string{"annotations-marker": "data-marker"}
	item := ProjectSecretListItem(rich)
	item.Metadata.Name, item.Metadata.Namespace, item.Metadata.UID = "secret-a", "team", "uid-a"
	resource := SecretResourceEvent{Type: "MODIFIED", ResourceType: SecretResourceType, Namespace: "team", WatcherSpecID: id, Resource: item}
	status := SecretWatcherStatus{WatcherSpecID: id, Status: WatchStatusConnected}
	watchErr := SecretWatcherError{WatcherSpecID: id, Code: WatchErrorResourceVersionExpired, Recoverable: true}
	for _, value := range []interface{}{SecretWatchSubscription{WatcherSpecID: id}, resource, status, watchErr} {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"value-marker", "data-marker", "labels-marker", "annotations-marker"} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatalf("watch DTO leaked %q: %s", forbidden, encoded)
			}
		}
	}
	converted := KubernetesSecretListItems([]SecretListItem{item})
	if len(converted) != 1 || converted[0].Metadata.Labels != nil || converted[0].Metadata.Annotations != nil {
		t.Fatalf("desktop conversion retained metadata maps: %#v", converted)
	}
	for _, invalid := range []SecretWatchSpecID{"", "short", SecretWatchSpecID(strings.Repeat("A", 42) + "=")} {
		if ValidSecretWatchSpecID(invalid) {
			t.Fatalf("invalid spec accepted: %q", invalid)
		}
	}
}
