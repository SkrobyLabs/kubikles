package acceleratorprovision

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
)

// SecretRPCClient is the closed desktop-only Accelerator Secret surface.
type SecretRPCClient interface {
	ListSecretsMetadata(context.Context, string, string, bool) ([]k8s.SecretListItem, error)
	GetSecretData(context.Context, string, string) ([]k8s.DataEntry, error)
	GetSecretYaml(context.Context, string, string) (string, error)
	CancelListRequest(context.Context, string) (bool, error)
	SubscribeSecretWatcher(context.Context, string, bool) (acceleratorsecret.SecretWatchSubscription, SecretWatchLease, error)
	UnsubscribeSecretWatcher(context.Context, acceleratorsecret.SecretWatchSpecID) error
	Done() <-chan struct{}
	Reason() acceleratorsecret.SecretClientReason
	Close(context.Context)
}

type secretRPCClient struct {
	lease       *SessionLease
	session     *ConnectedSession
	revoked     <-chan struct{}
	leaseClosed <-chan struct{}
	nonce       string
	done        chan struct{}
	detach      func()

	terminalOnce  sync.Once
	mu            sync.Mutex
	accepting     bool
	reason        acceleratorsecret.SecretClientReason
	callCounter   uint64
	listCounter   uint64
	calls         map[string]*secretPendingCall
	lists         map[string]*secretPendingCall
	subscriptions map[acceleratorsecret.SecretWatchSpecID]*secretWatchLease

	beforeRemoveCall func()
}

type secretPendingCall struct {
	id           string
	operation    acceleratorsecret.Operation
	result       chan secretCallResult
	callerListID string
	remoteListID string
	watcher      *secretWatchLease
}

type secretCallResult struct {
	frame acceleratorsecret.ResultFrame
}

func (secretCallResult) String() string { return "<accelerator secret call result>" }
func (secretCallResult) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator secret call result>")
}
func (secretCallResult) MarshalJSON() ([]byte, error) {
	return json.Marshal("<accelerator secret call result>")
}

type secretClientError struct {
	reason acceleratorsecret.SecretClientReason
}

func (e secretClientError) Error() string { return string(e.reason) }
func (e secretClientError) SecretClientReason() acceleratorsecret.SecretClientReason {
	return e.reason
}

func newSecretClientError(reason acceleratorsecret.SecretClientReason) error {
	return secretClientError{reason: reason}
}

func (c *secretRPCClient) logProtocolFailure(stage string, details map[string]interface{}) {
	if details == nil {
		details = make(map[string]interface{})
	}
	details["stage"] = stage
	if c != nil && c.session != nil {
		identity := c.session.Identity()
		details["context"] = identity.ContextName
		details["namespace"] = identity.ReleaseNamespace
		details["pod"] = identity.Pod.Name
	}
	debug.LogPortforward("Accelerator secret protocol failed", details)
}

// SecretClientReasonOf exposes only the closed reason enum needed by the
// desktop router. Untyped errors are deliberately not classified by text.
func SecretClientReasonOf(err error) (acceleratorsecret.SecretClientReason, bool) {
	type reasoned interface {
		SecretClientReason() acceleratorsecret.SecretClientReason
	}
	var failure reasoned
	if !errors.As(err, &failure) {
		return "", false
	}
	reason := failure.SecretClientReason()
	for _, allowed := range acceleratorsecret.SecretClientReasons() {
		if reason == allowed {
			return reason, true
		}
	}
	return "", false
}

func NewSecretRPCClient(lease *SessionLease) (SecretRPCClient, error) {
	return newSecretRPCClient(lease, rand.Reader)
}

func newSecretRPCClient(lease *SessionLease, entropy io.Reader) (*secretRPCClient, error) {
	session, revoked, leaseClosed, ok := lease.claimForSecretClient()
	if !ok || entropy == nil {
		return nil, newSecretClientError(acceleratorsecret.ReasonSessionUnavailable)
	}
	nonceBytes := make([]byte, 16)
	if _, err := io.ReadFull(entropy, nonceBytes); err != nil {
		lease.Close()
		return nil, newSecretClientError(acceleratorsecret.ReasonSessionUnavailable)
	}
	client := &secretRPCClient{
		lease: lease, session: session, revoked: revoked, leaseClosed: leaseClosed,
		nonce: base64.RawURLEncoding.EncodeToString(nonceBytes), done: make(chan struct{}), accepting: true,
		calls: make(map[string]*secretPendingCall), lists: make(map[string]*secretPendingCall), subscriptions: make(map[acceleratorsecret.SecretWatchSpecID]*secretWatchLease),
	}
	detach, attached := session.attachSecretFrameHandler(client.handleFrame)
	if !attached {
		lease.Close()
		return nil, newSecretClientError(acceleratorsecret.ReasonSessionUnavailable)
	}
	client.detach = detach
	go client.monitor()
	return client, nil
}

func (*secretRPCClient) String() string { return "<accelerator secret client>" }
func (*secretRPCClient) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator secret client>")
}

func (c *secretRPCClient) Done() <-chan struct{} {
	if c == nil {
		return closedCoordinatorSignal()
	}
	return c.done
}

func (c *secretRPCClient) Reason() acceleratorsecret.SecretClientReason {
	if c == nil {
		return acceleratorsecret.ReasonSessionUnavailable
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reason
}

func (c *secretRPCClient) Close(context.Context) {
	if c != nil {
		c.terminate(acceleratorsecret.ReasonClosed)
	}
}

func (c *secretRPCClient) monitor() {
	select {
	case <-c.revoked:
		c.terminate(acceleratorsecret.ReasonSessionUnavailable)
	case <-c.leaseClosed:
		c.terminate(acceleratorsecret.ReasonSessionUnavailable)
	case <-c.session.terminalStarted():
		reason := acceleratorsecret.ReasonSessionUnavailable
		if c.session.EndReason() == SessionProtocolFailed {
			reason = acceleratorsecret.ReasonProtocol
		}
		c.terminate(reason)
	case <-c.done:
	}
}

func (c *secretRPCClient) terminate(reason acceleratorsecret.SecretClientReason) {
	c.terminalOnce.Do(func() {
		var remoteLists []string
		var remoteSubscriptions []acceleratorsecret.SecretWatchSpecID
		var pendingCalls []*secretPendingCall
		var subscriptions []*secretWatchLease
		c.mu.Lock()
		c.accepting = false
		c.reason = reason
		for _, pending := range c.calls {
			pendingCalls = append(pendingCalls, pending)
			if pending.remoteListID != "" {
				remoteLists = append(remoteLists, pending.remoteListID)
			}
		}
		c.calls = make(map[string]*secretPendingCall)
		c.lists = make(map[string]*secretPendingCall)
		for id, subscription := range c.subscriptions {
			delete(c.subscriptions, id)
			remoteSubscriptions = append(remoteSubscriptions, id)
			subscriptions = append(subscriptions, subscription)
		}
		c.mu.Unlock()
		for _, pending := range pendingCalls {
			pending.result <- secretCallResult{frame: acceleratorsecret.ResultFrame{Status: acceleratorsecret.ResultStatusError, Reason: reason}}
		}
		for _, subscription := range subscriptions {
			subscription.terminateClearing(reason)
		}
		for _, remoteID := range remoteLists {
			c.bestEffortCancel(remoteID)
		}
		for _, id := range remoteSubscriptions {
			c.bestEffortUnsubscribe(id)
		}
		if c.detach != nil {
			c.detach()
		}
		if reason == acceleratorsecret.ReasonProtocol {
			c.session.beginTermination(SessionProtocolFailed, false)
		}
		c.lease.Close()
		close(c.done)
	})
}

func (c *secretRPCClient) handleFrame(frame acceleratorsecret.ServerFrame) bool {
	if frame.Result != nil {
		return c.handleResult(*frame.Result)
	}
	if frame.Event != nil {
		return c.handleWatchFrame(*frame.Event)
	}
	c.logProtocolFailure("validate_secret_frame", nil)
	return false
}

func (c *secretRPCClient) handleResult(frame acceleratorsecret.ResultFrame) bool {
	nonce, counter, ok := acceleratorsecret.ParseCallID(frame.ID)
	if !ok {
		c.logProtocolFailure("parse_result_call_id", map[string]interface{}{"callID": frame.ID})
		return false
	}
	if nonce != c.nonce {
		c.logProtocolFailure("validate_result_nonce", map[string]interface{}{"callID": frame.ID})
		return false
	}
	c.mu.Lock()
	if counter > c.callCounter {
		expectedCounter := c.callCounter
		c.mu.Unlock()
		c.logProtocolFailure("validate_result_counter", map[string]interface{}{"callID": frame.ID, "counter": counter, "latestCounter": expectedCounter})
		return false
	}
	pending := c.calls[frame.ID]
	if pending == nil {
		c.mu.Unlock()
		return true
	}
	delete(c.calls, frame.ID)
	if pending.callerListID != "" || pending.operation == acceleratorsecret.OperationListSecretsMetadata {
		if c.lists[pending.callerListID] == pending {
			delete(c.lists, pending.callerListID)
		}
	}
	if pending.watcher != nil && frame.Status == acceleratorsecret.ResultStatusError {
		delete(c.subscriptions, pending.watcher.subscription.WatcherSpecID)
		pending.watcher.terminate(frame.Reason)
	}
	owned := frame
	owned.Result = append(json.RawMessage(nil), frame.Result...)
	c.mu.Unlock()
	pending.result <- secretCallResult{frame: owned}
	return true
}
