package acceleratorsecret

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"kubikles/pkg/agent"
)

func TestSecretRPCOperationsExact(t *testing.T) {
	nonce := "AAAAAAAAAAAAAAAAAAAAAA"
	id, ok := CallID(nonce, 1)
	if !ok || id != "r."+nonce+".0000000000000001" {
		t.Fatalf("call ID=%q/%v", id, ok)
	}
	tests := []struct {
		operation Operation
		arity     int
		timeout   time.Duration
		encode    func() ([]byte, error)
	}{
		{OperationListSecretsMetadata, 3, 60 * time.Second, func() ([]byte, error) { return EncodeListSecretsMetadataCall(id, "request", "namespace", true) }},
		{OperationGetSecretData, 2, 30 * time.Second, func() ([]byte, error) { return EncodeGetSecretDataCall(id, "namespace", "name") }},
		{OperationGetSecretYAML, 2, 30 * time.Second, func() ([]byte, error) { return EncodeGetSecretYAMLCall(id, "namespace", "name") }},
		{OperationCancelListRequest, 1, 5 * time.Second, func() ([]byte, error) { return EncodeCancelListRequestCall(id, "request") }},
		{OperationSubscribeSecretWatcher, 2, 10 * time.Second, func() ([]byte, error) { return EncodeSubscribeSecretWatcherCall(id, "namespace", true) }},
		{OperationUnsubscribeSecretWatcher, 1, 5 * time.Second, func() ([]byte, error) { return EncodeUnsubscribeSecretWatcherCall(id, strings.Repeat("A", 43)) }},
	}
	if len(Operations()) != len(tests) {
		t.Fatalf("operations=%v", Operations())
	}
	for index, test := range tests {
		if Operations()[index] != test.operation {
			t.Fatalf("operation[%d]=%q", index, Operations()[index])
		}
		arity, ok := OperationArity(test.operation)
		if !ok || arity != test.arity || OperationTimeout(test.operation) != test.timeout {
			t.Fatalf("%s arity/timeout=%d/%s", test.operation, arity, OperationTimeout(test.operation))
		}
		payload, err := test.encode()
		if err != nil {
			t.Fatal(err)
		}
		call, err := DecodeCall(payload)
		if err != nil || call.Type != FrameTypeCall || call.ID != id || call.Operation != test.operation || len(call.Args) != test.arity {
			t.Fatalf("%s decoded=%#v err=%v", test.operation, call, err)
		}
	}
	policies := agent.V1MethodPolicies()
	if len(policies) != len(tests) {
		t.Fatalf("method policies=%v", policies)
	}
	for index, operation := range Operations() {
		if policies[index].Method != string(operation) {
			t.Fatalf("policy[%d]=%q, operation=%q", index, policies[index].Method, operation)
		}
		lookup, ok := agent.LookupMethodPolicy(string(operation))
		if !ok || lookup != policies[index] {
			t.Fatalf("policy lookup for %q=%#v/%v", operation, lookup, ok)
		}
	}
	for _, forbidden := range []Operation{"CreateSecret", "UpdateSecret", "DeleteSecret", "ListPods", "GetConfigMap", "CallMethod", "SubscribeWatcher"} {
		if _, ok := OperationArity(forbidden); ok {
			t.Fatalf("forbidden operation admitted: %q", forbidden)
		}
	}
	if MaxCreatorRequestFrameBytes != 64<<10 || MaxCreatorResponseFrameBytes != 8<<20 || MaxConcurrentCalls != 8 || OutboundSocketSlots != 64 || MaxSubscriptions != 32 || SubscriptionEventSlots != 64 {
		t.Fatal("fixed protocol limits changed")
	}
	wantReasons := []SecretClientReason{ReasonCanceled, ReasonDeadline, ReasonCapacity, ReasonForbidden, ReasonRemoteUnavailable, ReasonProtocol, ReasonWatchGap, ReasonSessionUnavailable, ReasonClosed}
	if got := SecretClientReasons(); len(got) != len(wantReasons) {
		t.Fatalf("reasons=%v", got)
	} else {
		for i := range got {
			if got[i] != wantReasons[i] {
				t.Fatalf("reason[%d]=%q", i, got[i])
			}
		}
	}
}

func TestSecretRPCStrictFramesAndIDs(t *testing.T) {
	nonce := "AAAAAAAAAAAAAAAAAAAAAA"
	first, _ := CallID(nonce, 1)
	second, _ := CallID(nonce, 2)
	sequence := CallIDSequence{}
	if !sequence.Accept(first) || !sequence.Accept(second) || sequence.Accept(second) || sequence.Accept(first) {
		t.Fatal("monotonic call sequence rejected or replayed")
	}
	foreign, _ := CallID("BBBBBBBBBBBBBBBBBBBBBB", 3)
	if sequence.Accept(foreign) {
		t.Fatal("foreign nonce accepted")
	}
	for _, invalid := range []string{"", "r.short.0000000000000001", "r.AAAAAAAAAAAAAAAAAAAAAA.0000000000000000", "r.AAAAAAAAAAAAAAAAAAAAAA.000000000000000A", "r.AAAAAAAAAAAAAAAAAAAAA=.0000000000000001", "r.AAAAAAAAAAAAAAAAAAAAAA.000000000000001"} {
		if _, _, ok := ParseCallID(invalid); ok {
			t.Fatalf("invalid call ID accepted: %q", invalid)
		}
	}
	validCall := `{"type":"call","id":"` + first + `","operation":"GetSecretData","args":["ns","name"]}`
	if _, err := DecodeCall([]byte(validCall)); err != nil {
		t.Fatal(err)
	}
	invalidCalls := []string{
		`{}`, strings.Replace(validCall, `"type":"call"`, `"type":"event"`, 1),
		strings.Replace(validCall, `"args":["ns","name"]`, `"args":["ns"]`, 1),
		strings.Replace(validCall, `"args":["ns","name"]`, `"args":["ns",1]`, 1),
		strings.Replace(validCall, `"operation":"GetSecretData"`, `"operation":"CreateSecret"`, 1),
		strings.TrimSuffix(validCall, "}") + `,"extra":true}`,
		strings.Replace(validCall, `"id":"`+first+`"`, `"id":"`+first+`","id":"`+first+`"`, 1),
		validCall + `{}`,
	}
	for _, payload := range invalidCalls {
		if _, err := DecodeCall([]byte(payload)); err == nil {
			t.Fatalf("invalid call accepted: %s", payload)
		}
	}
	okResult, err := EncodeResultOK(first, []string{"safe"})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeResult(okResult)
	if err != nil || decoded.Status != ResultStatusOK || decoded.Reason != "" || len(decoded.Result) == 0 {
		t.Fatalf("ok result=%#v err=%v", decoded, err)
	}
	errorResult, err := EncodeResultError(first, ReasonForbidden)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err = DecodeResult(errorResult)
	if err != nil || decoded.Status != ResultStatusError || decoded.Reason != ReasonForbidden || len(decoded.Result) != 0 {
		t.Fatalf("error result=%#v err=%v", decoded, err)
	}
	for _, payload := range [][]byte{
		[]byte(`{"type":"result","id":"` + first + `","status":"ok","reason":"forbidden","result":null}`),
		[]byte(`{"type":"result","id":"` + first + `","status":"error","reason":"forbidden","result":null}`),
		[]byte(`{"type":"result","id":"` + first + `","status":"error","reason":"future"}`),
		append(append([]byte(nil), okResult...), []byte(`{}`)...),
		bytes.Replace(okResult, []byte(`"status":"ok"`), []byte(`"status":"ok","status":"ok"`), 1),
	} {
		if _, err := DecodeResult(payload); err == nil {
			t.Fatalf("invalid result accepted: %s", payload)
		}
	}
	var roundTrip []string
	if err := DecodeResultValue(decoded.Result, &roundTrip); err == nil {
		t.Fatal("error result exposed a value")
	}
	var wire map[string]json.RawMessage
	if json.Unmarshal(okResult, &wire) != nil || len(wire) != 4 {
		t.Fatalf("unexpected result shape: %s", okResult)
	}
}

func TestSecretRPCStrictServerFrameDiscriminator(t *testing.T) {
	event := []byte(`{"type":"event","name":"watcher-status","data":{"watcherSpecId":"safe","status":"connected"}}`)
	decoded, err := DecodeServerFrame(event)
	if err != nil || decoded.Event == nil || decoded.Result != nil || decoded.Event.Name != EventWatcherStatus {
		t.Fatalf("event=%#v err=%v", decoded, err)
	}
	for _, invalid := range [][]byte{
		[]byte(`{"type":"connected","name":"watcher-status","data":null}`),
		[]byte(`{"type":"event","name":"watcher-status","data":null,"extra":true}`),
		[]byte(`{"type":"event","name":"watcher-status","name":"watcher-status","data":null}`),
		append(append([]byte(nil), event...), []byte(`{}`)...),
	} {
		if _, err := DecodeServerFrame(invalid); err == nil {
			t.Fatalf("invalid server frame accepted: %s", invalid)
		}
	}
}

func TestSecretRPCDecodedCarrierJSONRedactionPreservesWireVectors(t *testing.T) {
	hostile := json.RawMessage(`{"marker":"raw-private-marker"}`)
	result := ResultFrame{Type: FrameTypeResult, ID: "session-private-marker", Status: ResultStatusOK, Result: hostile}
	carriers := []struct {
		value interface{}
		want  string
	}{
		{CallFrame{Type: FrameTypeCall, ID: "session-private-marker", Operation: OperationGetSecretData, Args: []json.RawMessage{hostile}}, `"\u003caccelerator secret call frame\u003e"`},
		{result, `"\u003caccelerator secret result frame\u003e"`},
		{EventFrame{Type: "event", Name: EventResource, Data: hostile}, `"\u003caccelerator secret event frame\u003e"`},
		{ServerFrame{Result: &result}, `"\u003caccelerator secret server frame\u003e"`},
	}
	for _, carrier := range carriers {
		encoded, err := json.Marshal(carrier.value)
		if err != nil || string(encoded) != carrier.want || bytes.Contains(encoded, []byte("private-marker")) {
			t.Fatalf("carrier JSON=%s want=%s err=%v", encoded, carrier.want, err)
		}
	}

	id, _ := CallID("AAAAAAAAAAAAAAAAAAAAAA", 1)
	call, err := EncodeGetSecretDataCall(id, "namespace", "name")
	if err != nil || string(call) != `{"type":"call","id":"r.AAAAAAAAAAAAAAAAAAAAAA.0000000000000001","operation":"GetSecretData","args":["namespace","name"]}` {
		t.Fatalf("call vector=%s err=%v", call, err)
	}
	resultWire, err := EncodeResultOK(id, "safe")
	if err != nil || string(resultWire) != `{"type":"result","id":"r.AAAAAAAAAAAAAAAAAAAAAA.0000000000000001","status":"ok","result":"safe"}` {
		t.Fatalf("result vector=%s err=%v", resultWire, err)
	}
	errorWire, err := EncodeResultError(id, ReasonForbidden)
	if err != nil || string(errorWire) != `{"type":"result","id":"r.AAAAAAAAAAAAAAAAAAAAAA.0000000000000001","status":"error","reason":"forbidden"}` {
		t.Fatalf("error vector=%s err=%v", errorWire, err)
	}
}
