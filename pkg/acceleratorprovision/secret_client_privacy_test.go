package acceleratorprovision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"testing"

	"kubikles/pkg/acceleratorsecret"
)

func TestSecretClientPrivacyAndOwnedSourceBoundary(t *testing.T) {
	client, socket, lease, _ := secretClientFixture(t)
	const detailMarker = "explicit-detail-private-marker"
	detailDone := make(chan []string, 1)
	detailErr := make(chan error, 1)
	go func() {
		entries, err := client.GetSecretData(context.Background(), "private-namespace-marker", "private-name-marker")
		values := make([]string, len(entries))
		for index := range entries {
			values[index] = entries[index].Value
		}
		detailDone <- values
		detailErr <- err
	}()
	detailCall := readSecretCall(t, socket)
	sendSecretResult(t, socket, detailCall.ID, []map[string]string{{"key": "key-private-marker", "value": detailMarker}})
	if err := <-detailErr; err != nil {
		t.Fatal(err)
	}
	if values := <-detailDone; len(values) != 1 || values[0] != detailMarker {
		t.Fatalf("explicit detail result=%q", values)
	}
	yamlDone := make(chan string, 1)
	yamlErr := make(chan error, 1)
	go func() {
		value, err := client.GetSecretYaml(context.Background(), "private-namespace-marker", "private-name-marker")
		yamlDone <- value
		yamlErr <- err
	}()
	yamlCall := readSecretCall(t, socket)
	sendSecretResult(t, socket, yamlCall.ID, "yaml-explicit-detail-private-marker")
	if err := <-yamlErr; err != nil {
		t.Fatal(err)
	}
	if value := <-yamlDone; value != "yaml-explicit-detail-private-marker" {
		t.Fatalf("explicit YAML result=%q", value)
	}

	client.session.identity.SessionID = "session-private-marker"
	client.session.identity.InstanceID = "instance-private-marker"
	client.session.identity.WorkloadSessionID = "workload-private-marker"
	client.session.identity.ContextName = "context-private-marker"
	markers := []string{
		client.nonce,
		client.session.identity.SessionID,
		client.session.identity.InstanceID,
		client.session.identity.WorkloadSessionID,
		"Authorization: raw-private-marker",
		"http://127.0.0.1:43123/private-marker",
		"kind: Secret\ndata: private-value-marker",
		"verifier-private-marker",
		"generation-private-marker",
		"request-args-private-marker",
		"raw-frame-private-marker",
		"raw-error-private-marker",
		"key-private-marker",
		detailMarker,
		"yaml-explicit-detail-private-marker",
	}
	watcher := &secretWatchLease{
		client:       client,
		subscription: acceleratorsecret.SecretWatchSubscription{WatcherSpecID: acceleratorsecret.SecretWatchSpecIDFor("private-namespace-marker", false)},
		namespace:    "private-namespace-marker",
		events:       make(chan SecretWatchEvent, 1),
		done:         make(chan struct{}),
	}
	var resource acceleratorsecret.SecretResourceEvent
	resource.Namespace = "private-namespace-marker"
	resource.Resource.Metadata.Name = "private-name-marker"
	event := SecretWatchEvent{resource: &resource}
	rawPrivate := json.RawMessage(`{"marker":"raw-frame-private-marker","value":"private-value-marker","authorization":"Authorization: raw-private-marker","verifier":"verifier-private-marker","generation":"generation-private-marker","args":"request-args-private-marker","error":"raw-error-private-marker"}`)
	result := secretCallResult{frame: acceleratorsecret.ResultFrame{ID: "session-private-marker", Status: acceleratorsecret.ResultStatusOK, Result: rawPrivate}}
	wireCall := acceleratorsecret.CallFrame{ID: "session-private-marker", Operation: acceleratorsecret.OperationGetSecretData, Args: []json.RawMessage{rawPrivate}}
	wireResult := acceleratorsecret.ResultFrame{ID: "workload-private-marker", Status: acceleratorsecret.ResultStatusOK, Result: rawPrivate}
	wireEvent := acceleratorsecret.EventFrame{Name: acceleratorsecret.EventResource, Data: rawPrivate}
	wireServer := acceleratorsecret.ServerFrame{Result: &wireResult}
	clientJSON, _ := json.Marshal(client)
	watchJSON, _ := json.Marshal(watcher)
	eventJSON, _ := json.Marshal(event)
	leaseJSON, _ := json.Marshal(lease)
	resultJSON, _ := json.Marshal(result)
	wireCallJSON, _ := json.Marshal(wireCall)
	wireResultJSON, _ := json.Marshal(wireResult)
	wireEventJSON, _ := json.Marshal(wireEvent)
	wireServerJSON, _ := json.Marshal(wireServer)
	for _, redacted := range []struct {
		got  []byte
		want string
	}{
		{wireCallJSON, `"\u003caccelerator secret call frame\u003e"`},
		{wireResultJSON, `"\u003caccelerator secret result frame\u003e"`},
		{wireEventJSON, `"\u003caccelerator secret event frame\u003e"`},
		{wireServerJSON, `"\u003caccelerator secret server frame\u003e"`},
	} {
		if string(redacted.got) != redacted.want || bytes.Contains(redacted.got, []byte("private-marker")) {
			t.Fatalf("decoded carrier JSON=%s want=%s", redacted.got, redacted.want)
		}
	}
	var capturedLogs bytes.Buffer
	priorLogWriter, priorLogFlags, priorLogPrefix := log.Writer(), log.Flags(), log.Prefix()
	log.SetOutput(&capturedLogs)
	log.SetFlags(0)
	log.SetPrefix("")
	defer func() {
		log.SetOutput(priorLogWriter)
		log.SetFlags(priorLogFlags)
		log.SetPrefix(priorLogPrefix)
	}()
	client.terminate(acceleratorsecret.ReasonRemoteUnavailable)
	corpus := []string{
		fmt.Sprintf("%s %+v %#v", client, client, client), string(clientJSON),
		fmt.Sprintf("%s %+v %#v", watcher, watcher, watcher), string(watchJSON),
		fmt.Sprintf("%s %+v %#v", event, event, event), string(eventJSON), string(leaseJSON),
		fmt.Sprintf("%s %+v %#v", result, result, result), string(resultJSON),
		fmt.Sprintf("%s %+v %#v", wireCall, wireCall, wireCall),
		fmt.Sprintf("%s %+v %#v", wireResult, wireResult, wireResult),
		fmt.Sprintf("%s %+v %#v", wireEvent, wireEvent, wireEvent),
		fmt.Sprintf("%s %+v %#v", wireServer, wireServer, wireServer),
		string(wireCallJSON), string(wireResultJSON), string(wireEventJSON), string(wireServerJSON),
		newSecretClientError(acceleratorsecret.ReasonRemoteUnavailable).Error(),
		capturedLogs.String(),
	}
	for _, rendered := range corpus {
		for _, marker := range markers {
			if marker != "" && strings.Contains(rendered, marker) {
				t.Fatalf("privacy marker %q escaped through %q", marker, rendered)
			}
		}
		for _, marker := range []string{"private-namespace-marker", "private-name-marker", "raw-private-marker", "private-value-marker"} {
			if strings.Contains(rendered, marker) {
				t.Fatalf("private result marker %q escaped through %q", marker, rendered)
			}
		}
	}
	if got := fmt.Sprint(newSecretClientError(acceleratorsecret.ReasonRemoteUnavailable)); got != string(acceleratorsecret.ReasonRemoteUnavailable) {
		t.Fatalf("safe error=%q", got)
	}
	client.Close(context.Background())
}
