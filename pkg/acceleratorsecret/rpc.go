package acceleratorsecret

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"
)

const (
	FrameTypeCall   = "call"
	FrameTypeResult = "result"

	MaxCreatorRequestFrameBytes  = 64 << 10
	MaxCreatorResponseFrameBytes = 8 << 20
	MaxConcurrentCalls           = 8
	OutboundSocketSlots          = 64
	MaxSubscriptions             = 32
	SubscriptionEventSlots       = 64
)

type Operation string

const (
	OperationListSecretsMetadata      Operation = "ListSecretsMetadata"
	OperationGetSecretData            Operation = "GetSecretData"
	OperationGetSecretYAML            Operation = "GetSecretYaml"
	OperationCancelListRequest        Operation = "CancelListRequest"
	OperationSubscribeSecretWatcher   Operation = "SubscribeSecretWatcher"
	OperationUnsubscribeSecretWatcher Operation = "UnsubscribeSecretWatcher"
)

var operations = []Operation{
	OperationListSecretsMetadata,
	OperationGetSecretData,
	OperationGetSecretYAML,
	OperationCancelListRequest,
	OperationSubscribeSecretWatcher,
	OperationUnsubscribeSecretWatcher,
}

func Operations() []Operation { return append([]Operation(nil), operations...) }

func OperationArity(operation Operation) (int, bool) {
	switch operation {
	case OperationListSecretsMetadata:
		return 3, true
	case OperationGetSecretData, OperationGetSecretYAML, OperationSubscribeSecretWatcher:
		return 2, true
	case OperationCancelListRequest, OperationUnsubscribeSecretWatcher:
		return 1, true
	default:
		return 0, false
	}
}

func OperationTimeout(operation Operation) time.Duration {
	switch operation {
	case OperationListSecretsMetadata:
		return 60 * time.Second
	case OperationGetSecretData, OperationGetSecretYAML:
		return 30 * time.Second
	case OperationSubscribeSecretWatcher:
		return 10 * time.Second
	case OperationCancelListRequest, OperationUnsubscribeSecretWatcher:
		return 5 * time.Second
	default:
		return 0
	}
}

type SecretClientReason string

const (
	ReasonCanceled           SecretClientReason = "canceled"
	ReasonDeadline           SecretClientReason = "deadline"
	ReasonCapacity           SecretClientReason = "capacity"
	ReasonForbidden          SecretClientReason = "forbidden"
	ReasonRemoteUnavailable  SecretClientReason = "remote_unavailable"
	ReasonProtocol           SecretClientReason = "protocol"
	ReasonWatchGap           SecretClientReason = "watch_gap"
	ReasonSessionUnavailable SecretClientReason = "session_unavailable"
	ReasonClosed             SecretClientReason = "closed"
)

var clientReasons = []SecretClientReason{ReasonCanceled, ReasonDeadline, ReasonCapacity, ReasonForbidden, ReasonRemoteUnavailable, ReasonProtocol, ReasonWatchGap, ReasonSessionUnavailable, ReasonClosed}

func SecretClientReasons() []SecretClientReason {
	return append([]SecretClientReason(nil), clientReasons...)
}

func validReason(reason SecretClientReason) bool {
	for _, candidate := range clientReasons {
		if candidate == reason {
			return true
		}
	}
	return false
}

func canonicalNonce(nonce string) bool {
	if len(nonce) != 22 {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(nonce)
	return err == nil && len(decoded) == 16 && base64.RawURLEncoding.EncodeToString(decoded) == nonce
}

func CallID(nonce string, counter uint64) (string, bool) {
	if !canonicalNonce(nonce) || counter == 0 {
		return "", false
	}
	return "r." + nonce + "." + fmt.Sprintf("%016x", counter), true
}

func ParseCallID(id string) (nonce string, counter uint64, ok bool) {
	if len(id) != 41 || id[:2] != "r." || id[24] != '.' {
		return "", 0, false
	}
	nonce = id[2:24]
	counterText := id[25:]
	if !canonicalNonce(nonce) || len(counterText) != 16 {
		return "", 0, false
	}
	for _, value := range []byte(counterText) {
		if !((value >= '0' && value <= '9') || (value >= 'a' && value <= 'f')) {
			return "", 0, false
		}
	}
	decoded, err := hex.DecodeString(counterText)
	if err != nil || len(decoded) != 8 {
		return "", 0, false
	}
	counter, err = strconv.ParseUint(counterText, 16, 64)
	return nonce, counter, err == nil && counter != 0
}

type CallIDSequence struct {
	mu      sync.Mutex
	nonce   string
	counter uint64
}

func (s *CallIDSequence) Accept(id string) bool {
	if s == nil {
		return false
	}
	nonce, counter, ok := ParseCallID(id)
	if !ok {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nonce == "" {
		s.nonce = nonce
	} else if s.nonce != nonce {
		return false
	}
	if counter <= s.counter {
		return false
	}
	s.counter = counter
	return true
}

type CallFrame struct {
	Type      string
	ID        string
	Operation Operation
	Args      []json.RawMessage
}

func (CallFrame) String() string { return "<accelerator secret call frame>" }
func (CallFrame) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator secret call frame>")
}
func (CallFrame) MarshalJSON() ([]byte, error) {
	return json.Marshal("<accelerator secret call frame>")
}

type ResultStatus string

const (
	ResultStatusOK    ResultStatus = "ok"
	ResultStatusError ResultStatus = "error"
)

type ResultFrame struct {
	Type   string
	ID     string
	Status ResultStatus
	Result json.RawMessage
	Reason SecretClientReason
}

func (ResultFrame) String() string { return "<accelerator secret result frame>" }
func (ResultFrame) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator secret result frame>")
}
func (ResultFrame) MarshalJSON() ([]byte, error) {
	return json.Marshal("<accelerator secret result frame>")
}

type EventFrame struct {
	Type string
	Name string
	Data json.RawMessage
}

func (EventFrame) String() string { return "<accelerator secret event frame>" }
func (EventFrame) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator secret event frame>")
}
func (EventFrame) MarshalJSON() ([]byte, error) {
	return json.Marshal("<accelerator secret event frame>")
}

type ServerFrame struct {
	Result *ResultFrame
	Event  *EventFrame
}

func (ServerFrame) String() string { return "<accelerator secret server frame>" }
func (ServerFrame) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator secret server frame>")
}
func (ServerFrame) MarshalJSON() ([]byte, error) {
	return json.Marshal("<accelerator secret server frame>")
}

func encodeCall(id string, operation Operation, values ...interface{}) ([]byte, error) {
	if _, _, ok := ParseCallID(id); !ok {
		return nil, errors.New("invalid call")
	}
	arity, ok := OperationArity(operation)
	if !ok || len(values) != arity {
		return nil, errors.New("invalid call")
	}
	args := make([]json.RawMessage, len(values))
	for i := range values {
		encoded, err := json.Marshal(values[i])
		if err != nil {
			return nil, errors.New("invalid call")
		}
		args[i] = encoded
	}
	wire := struct {
		Type      string            `json:"type"`
		ID        string            `json:"id"`
		Operation Operation         `json:"operation"`
		Args      []json.RawMessage `json:"args"`
	}{FrameTypeCall, id, operation, args}
	return json.Marshal(wire)
}

func EncodeListSecretsMetadataCall(id, requestID, namespace string, excludeHelmReleases bool) ([]byte, error) {
	return encodeCall(id, OperationListSecretsMetadata, requestID, namespace, excludeHelmReleases)
}
func EncodeGetSecretDataCall(id, namespace, name string) ([]byte, error) {
	return encodeCall(id, OperationGetSecretData, namespace, name)
}
func EncodeGetSecretYAMLCall(id, namespace, name string) ([]byte, error) {
	return encodeCall(id, OperationGetSecretYAML, namespace, name)
}
func EncodeCancelListRequestCall(id, requestID string) ([]byte, error) {
	return encodeCall(id, OperationCancelListRequest, requestID)
}
func EncodeSubscribeSecretWatcherCall(id, namespace string, excludeHelmReleases bool) ([]byte, error) {
	return encodeCall(id, OperationSubscribeSecretWatcher, namespace, excludeHelmReleases)
}
func EncodeUnsubscribeSecretWatcherCall(id, watcherSpecID string) ([]byte, error) {
	return encodeCall(id, OperationUnsubscribeSecretWatcher, watcherSpecID)
}

func DecodeCall(payload []byte) (CallFrame, error) {
	var frame CallFrame
	if len(payload) == 0 || len(payload) > MaxCreatorRequestFrameBytes {
		return frame, errors.New("invalid call")
	}
	fields, err := strictObject(payload, map[string]struct{}{"type": {}, "id": {}, "operation": {}, "args": {}})
	if err != nil || len(fields) != 4 || decodeCanonicalString(fields["type"], &frame.Type) != nil || decodeCanonicalString(fields["id"], &frame.ID) != nil {
		return frame, errors.New("invalid call")
	}
	var operation string
	if decodeCanonicalString(fields["operation"], &operation) != nil || json.Unmarshal(fields["args"], &frame.Args) != nil || frame.Args == nil {
		return frame, errors.New("invalid call")
	}
	frame.Operation = Operation(operation)
	if frame.Type != FrameTypeCall {
		return frame, errors.New("invalid call")
	}
	if _, _, ok := ParseCallID(frame.ID); !ok {
		return frame, errors.New("invalid call")
	}
	if err := validateArgs(frame.Operation, frame.Args); err != nil {
		return frame, err
	}
	return frame, nil
}

func validateArgs(operation Operation, args []json.RawMessage) error {
	arity, ok := OperationArity(operation)
	if !ok || len(args) != arity {
		return errors.New("invalid call")
	}
	for index := range args {
		if len(args[index]) == 0 || bytes.Equal(args[index], []byte("null")) {
			return errors.New("invalid call")
		}
		wantBool := (operation == OperationListSecretsMetadata && index == 2) || (operation == OperationSubscribeSecretWatcher && index == 1)
		if wantBool {
			var value bool
			if json.Unmarshal(args[index], &value) != nil || (!bytes.Equal(args[index], []byte("true")) && !bytes.Equal(args[index], []byte("false"))) {
				return errors.New("invalid call")
			}
			continue
		}
		var value string
		if decodeCanonicalString(args[index], &value) != nil {
			return errors.New("invalid call")
		}
	}
	return nil
}

func EncodeResultOK(id string, value interface{}) ([]byte, error) {
	if _, _, ok := ParseCallID(id); !ok {
		return nil, errors.New("invalid result")
	}
	result, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("invalid result")
	}
	wire := struct {
		Type   string          `json:"type"`
		ID     string          `json:"id"`
		Status ResultStatus    `json:"status"`
		Result json.RawMessage `json:"result"`
	}{FrameTypeResult, id, ResultStatusOK, result}
	return json.Marshal(wire)
}

func EncodeResultError(id string, reason SecretClientReason) ([]byte, error) {
	if _, _, ok := ParseCallID(id); !ok || !validReason(reason) {
		return nil, errors.New("invalid result")
	}
	wire := struct {
		Type   string             `json:"type"`
		ID     string             `json:"id"`
		Status ResultStatus       `json:"status"`
		Reason SecretClientReason `json:"reason"`
	}{FrameTypeResult, id, ResultStatusError, reason}
	return json.Marshal(wire)
}

func DecodeResult(payload []byte) (ResultFrame, error) {
	decoded, err := DecodeServerFrame(payload)
	if err != nil || decoded.Result == nil {
		return ResultFrame{}, errors.New("invalid result")
	}
	return *decoded.Result, nil
}

func DecodeEvent(payload []byte) (EventFrame, error) {
	decoded, err := DecodeServerFrame(payload)
	if err != nil || decoded.Event == nil {
		return EventFrame{}, errors.New("invalid event")
	}
	return *decoded.Event, nil
}

// DecodeServerFrame decodes the post-handshake server discriminator exactly
// once and admits only a strict result or existing event envelope.
func DecodeServerFrame(payload []byte) (ServerFrame, error) {
	var decoded ServerFrame
	if len(payload) == 0 || len(payload) > MaxCreatorResponseFrameBytes {
		return decoded, errors.New("invalid server frame")
	}
	fields, err := strictObject(payload, map[string]struct{}{"type": {}, "id": {}, "status": {}, "result": {}, "reason": {}, "name": {}, "data": {}})
	var frameType string
	if err != nil || decodeCanonicalString(fields["type"], &frameType) != nil {
		return decoded, errors.New("invalid server frame")
	}
	if frameType == "event" {
		var event EventFrame
		if len(fields) != 3 || decodeCanonicalString(fields["name"], &event.Name) != nil || !json.Valid(fields["data"]) {
			return decoded, errors.New("invalid server frame")
		}
		event.Type = frameType
		event.Data = append(json.RawMessage(nil), fields["data"]...)
		decoded.Event = &event
		return decoded, nil
	}
	if frameType != FrameTypeResult {
		return decoded, errors.New("invalid server frame")
	}
	frame, err := decodeResultFields(fields)
	if err != nil {
		return decoded, err
	}
	decoded.Result = &frame
	return decoded, nil
}

func decodeResultFields(fields map[string]json.RawMessage) (ResultFrame, error) {
	var frame ResultFrame
	if decodeCanonicalString(fields["type"], &frame.Type) != nil || decodeCanonicalString(fields["id"], &frame.ID) != nil {
		return frame, errors.New("invalid result")
	}
	var status string
	if decodeCanonicalString(fields["status"], &status) != nil || frame.Type != FrameTypeResult {
		return frame, errors.New("invalid result")
	}
	frame.Status = ResultStatus(status)
	if _, _, ok := ParseCallID(frame.ID); !ok {
		return frame, errors.New("invalid result")
	}
	result, hasResult := fields["result"]
	reasonRaw, hasReason := fields["reason"]
	switch frame.Status {
	case ResultStatusOK:
		if !hasResult || hasReason || !json.Valid(result) || len(fields) != 4 {
			return frame, errors.New("invalid result")
		}
		frame.Result = append(json.RawMessage(nil), result...)
	case ResultStatusError:
		var reason string
		if hasResult || !hasReason || len(fields) != 4 || decodeCanonicalString(reasonRaw, &reason) != nil {
			return frame, errors.New("invalid result")
		}
		frame.Reason = SecretClientReason(reason)
		if !validReason(frame.Reason) {
			return frame, errors.New("invalid result")
		}
	default:
		return frame, errors.New("invalid result")
	}
	return frame, nil
}

func DecodeResultValue(raw json.RawMessage, target interface{}) error {
	if len(raw) == 0 || target == nil {
		return errors.New("invalid result")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("invalid result")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("invalid result")
	}
	return nil
}

func decodeCanonicalString(raw json.RawMessage, target *string) error {
	if len(raw) == 0 || target == nil || json.Unmarshal(raw, target) != nil {
		return errors.New("invalid string")
	}
	canonical, _ := json.Marshal(*target)
	if !bytes.Equal(raw, canonical) {
		return errors.New("invalid string")
	}
	return nil
}

func strictObject(payload []byte, allowed map[string]struct{}) (map[string]json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("invalid object")
	}
	fields := make(map[string]json.RawMessage, len(allowed))
	for decoder.More() {
		token, err = decoder.Token()
		name, ok := token.(string)
		if err != nil || !ok {
			return nil, errors.New("invalid object")
		}
		if _, ok := allowed[name]; !ok {
			return nil, errors.New("invalid object")
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, errors.New("invalid object")
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, errors.New("invalid object")
		}
		fields[name] = append(json.RawMessage(nil), value...)
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, errors.New("invalid object")
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("invalid object")
	}
	return fields, nil
}
