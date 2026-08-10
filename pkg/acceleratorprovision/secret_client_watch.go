package acceleratorprovision

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"kubikles/pkg/acceleratorsecret"
)

type SecretWatchEvent struct {
	resource   *acceleratorsecret.SecretResourceEvent
	status     *acceleratorsecret.SecretWatcherStatus
	watchError *acceleratorsecret.SecretWatcherError
}

func (SecretWatchEvent) String() string { return "<accelerator secret watch event>" }
func (SecretWatchEvent) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator secret watch event>")
}
func (SecretWatchEvent) MarshalJSON() ([]byte, error) {
	return json.Marshal("<accelerator secret watch event>")
}

func (e SecretWatchEvent) Resource() (acceleratorsecret.SecretResourceEvent, bool) {
	if e.resource == nil {
		return acceleratorsecret.SecretResourceEvent{}, false
	}
	return *e.resource, true
}

func (e SecretWatchEvent) Status() (acceleratorsecret.SecretWatcherStatus, bool) {
	if e.status == nil {
		return acceleratorsecret.SecretWatcherStatus{}, false
	}
	return *e.status, true
}

func (e SecretWatchEvent) Error() (acceleratorsecret.SecretWatcherError, bool) {
	if e.watchError == nil {
		return acceleratorsecret.SecretWatcherError{}, false
	}
	return *e.watchError, true
}

type SecretWatchLease interface {
	Subscription() acceleratorsecret.SecretWatchSubscription
	Events() <-chan SecretWatchEvent
	Done() <-chan struct{}
	Reason() acceleratorsecret.SecretClientReason
	Close(context.Context)
}

type secretWatchLease struct {
	client       *secretRPCClient
	subscription acceleratorsecret.SecretWatchSubscription
	namespace    string
	excludeHelm  bool
	events       chan SecretWatchEvent
	done         chan struct{}

	once   sync.Once
	mu     sync.Mutex
	reason acceleratorsecret.SecretClientReason
	active bool
	closed bool
}

func (*secretWatchLease) String() string { return "<accelerator secret watch lease>" }
func (*secretWatchLease) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator secret watch lease>")
}

func (l *secretWatchLease) Subscription() acceleratorsecret.SecretWatchSubscription {
	if l == nil {
		return acceleratorsecret.SecretWatchSubscription{}
	}
	return l.subscription
}

func (l *secretWatchLease) Events() <-chan SecretWatchEvent {
	if l == nil {
		closed := make(chan SecretWatchEvent)
		close(closed)
		return closed
	}
	return l.events
}

func (l *secretWatchLease) Done() <-chan struct{} {
	if l == nil {
		return closedCoordinatorSignal()
	}
	return l.done
}

func (l *secretWatchLease) Reason() acceleratorsecret.SecretClientReason {
	if l == nil {
		return acceleratorsecret.ReasonSessionUnavailable
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.reason
}

func (l *secretWatchLease) Close(ctx context.Context) {
	if l != nil && l.client != nil {
		_ = l.client.UnsubscribeSecretWatcher(ctx, l.subscription.WatcherSpecID)
	}
}

func (l *secretWatchLease) terminate(reason acceleratorsecret.SecretClientReason) {
	l.terminateOwned(reason, false)
}

func (l *secretWatchLease) terminateClearing(reason acceleratorsecret.SecretClientReason) {
	l.terminateOwned(reason, true)
}

func (l *secretWatchLease) terminateOwned(reason acceleratorsecret.SecretClientReason, clearQueued bool) {
	if l == nil {
		return
	}
	l.once.Do(func() {
		l.mu.Lock()
		l.reason = reason
		l.closed = true
		if !clearQueued {
			close(l.events)
			close(l.done)
			l.mu.Unlock()
			return
		}
		for {
			select {
			case event := <-l.events:
				clearSecretWatchEvent(&event)
			default:
				close(l.events)
				close(l.done)
				l.mu.Unlock()
				return
			}
		}
	})
}

func clearSecretWatchEvent(event *SecretWatchEvent) {
	if event == nil {
		return
	}
	if event.resource != nil {
		*event.resource = acceleratorsecret.SecretResourceEvent{}
	}
	if event.status != nil {
		*event.status = acceleratorsecret.SecretWatcherStatus{}
	}
	if event.watchError != nil {
		*event.watchError = acceleratorsecret.SecretWatcherError{}
	}
	*event = SecretWatchEvent{}
}

func (l *secretWatchLease) deliver(event SecretWatchEvent) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return true
	}
	select {
	case l.events <- event:
		return true
	default:
		return false
	}
}

func (c *secretRPCClient) SubscribeSecretWatcher(ctx context.Context, namespace string, excludeHelmReleases bool) (acceleratorsecret.SecretWatchSubscription, SecretWatchLease, error) {
	subscription := acceleratorsecret.SecretWatchSubscription{WatcherSpecID: acceleratorsecret.SecretWatchSpecIDFor(namespace, excludeHelmReleases)}
	watcher := &secretWatchLease{client: c, subscription: subscription, namespace: namespace, excludeHelm: excludeHelmReleases, events: make(chan SecretWatchEvent, acceleratorsecret.SubscriptionEventSlots), done: make(chan struct{})}
	pending, err := c.startWatchCall(watcher, namespace, excludeHelmReleases)
	if err != nil {
		watcher.terminate(reasonFromError(err))
		return acceleratorsecret.SecretWatchSubscription{}, nil, err
	}
	frame, err := c.waitCall(ctx, pending)
	if err != nil {
		c.removeSubscription(watcher)
		watcher.terminate(reasonFromError(err))
		c.bestEffortUnsubscribe(subscription.WatcherSpecID)
		return acceleratorsecret.SecretWatchSubscription{}, nil, err
	}
	defer clear(frame.Result)
	var confirmed acceleratorsecret.SecretWatchSubscription
	decodeErr := acceleratorsecret.DecodeResultValue(frame.Result, &confirmed)
	if decodeErr != nil || confirmed.WatcherSpecID != subscription.WatcherSpecID {
		details := map[string]interface{}{"payloadBytes": len(frame.Result), "watcherSpecID": subscription.WatcherSpecID, "confirmedWatcherSpecID": confirmed.WatcherSpecID}
		if decodeErr != nil {
			details["error"] = decodeErr.Error()
		}
		c.logProtocolFailure("validate_subscribe_result", details)
		c.removeSubscription(watcher)
		watcher.terminate(acceleratorsecret.ReasonProtocol)
		c.terminate(acceleratorsecret.ReasonProtocol)
		return acceleratorsecret.SecretWatchSubscription{}, nil, newSecretClientError(acceleratorsecret.ReasonProtocol)
	}
	watcher.mu.Lock()
	if watcher.closed {
		watcher.mu.Unlock()
		return acceleratorsecret.SecretWatchSubscription{}, nil, newSecretClientError(watcher.Reason())
	}
	watcher.active = true
	watcher.mu.Unlock()
	return subscription, watcher, nil
}

func (c *secretRPCClient) UnsubscribeSecretWatcher(ctx context.Context, id acceleratorsecret.SecretWatchSpecID) error {
	c.mu.Lock()
	watcher := c.subscriptions[id]
	if watcher == nil {
		c.mu.Unlock()
		return nil
	}
	delete(c.subscriptions, id)
	c.mu.Unlock()
	watcher.terminate(acceleratorsecret.ReasonClosed)
	pending, err := c.startCall(acceleratorsecret.OperationUnsubscribeSecretWatcher, func(callID string) ([]byte, error) {
		return acceleratorsecret.EncodeUnsubscribeSecretWatcherCall(callID, string(id))
	})
	if err != nil {
		return err
	}
	frame, err := c.waitCall(ctx, pending)
	if err != nil {
		return err
	}
	defer clear(frame.Result)
	if !bytes.Equal(frame.Result, []byte("null")) {
		c.logProtocolFailure("validate_unsubscribe_result", map[string]interface{}{"payloadBytes": len(frame.Result), "watcherSpecID": id})
		c.terminate(acceleratorsecret.ReasonProtocol)
		return newSecretClientError(acceleratorsecret.ReasonProtocol)
	}
	return nil
}

func (c *secretRPCClient) startWatchCall(watcher *secretWatchLease, namespace string, exclude bool) (*secretPendingCall, error) {
	c.mu.Lock()
	if !c.accepting {
		reason := c.reason
		c.mu.Unlock()
		return nil, newSecretClientError(reason)
	}
	if len(c.calls) >= acceleratorsecret.MaxConcurrentCalls || len(c.subscriptions) >= acceleratorsecret.MaxSubscriptions {
		c.mu.Unlock()
		return nil, newSecretClientError(acceleratorsecret.ReasonCapacity)
	}
	id := watcher.subscription.WatcherSpecID
	if _, duplicate := c.subscriptions[id]; duplicate {
		c.mu.Unlock()
		return nil, newSecretClientError(acceleratorsecret.ReasonCapacity)
	}
	c.callCounter++
	callID, ok := acceleratorsecret.CallID(c.nonce, c.callCounter)
	if !ok {
		c.mu.Unlock()
		return nil, newSecretClientError(acceleratorsecret.ReasonProtocol)
	}
	pending := &secretPendingCall{id: callID, operation: acceleratorsecret.OperationSubscribeSecretWatcher, result: make(chan secretCallResult, 1), watcher: watcher}
	c.calls[callID] = pending
	c.subscriptions[id] = watcher
	payload, err := acceleratorsecret.EncodeSubscribeSecretWatcherCall(callID, namespace, exclude)
	sendReason := acceleratorsecret.ReasonProtocol
	if err == nil {
		sendReason = c.session.sendApplicationFrame(payload)
	}
	if err != nil || sendReason != "" {
		delete(c.calls, callID)
		delete(c.subscriptions, id)
		c.mu.Unlock()
		clear(payload)
		return nil, newSecretClientError(sendReason)
	}
	c.mu.Unlock()
	clear(payload)
	return pending, nil
}

func (c *secretRPCClient) handleWatchFrame(frame acceleratorsecret.EventFrame) bool {
	switch frame.Name {
	case acceleratorsecret.EventResource:
		var resource acceleratorsecret.SecretResourceEvent
		if decodeErr := acceleratorsecret.DecodeResultValue(frame.Data, &resource); decodeErr != nil {
			c.logProtocolFailure("decode_resource_event", map[string]interface{}{"error": decodeErr.Error(), "payloadBytes": len(frame.Data)})
			return false
		}
		c.mu.Lock()
		watcher := c.subscriptions[resource.WatcherSpecID]
		c.mu.Unlock()
		if watcher == nil {
			return true
		}
		namespaceMismatch := resource.Namespace != resource.Resource.Metadata.Namespace || (watcher.namespace != "" && resource.Namespace != watcher.namespace)
		if resource.ResourceType != acceleratorsecret.SecretResourceType || namespaceMismatch || (resource.Type != "ADDED" && resource.Type != "MODIFIED" && resource.Type != "DELETED") {
			c.failSubscription(watcher, acceleratorsecret.ReasonWatchGap)
			return true
		}
		if !watcher.deliver(SecretWatchEvent{resource: &resource}) {
			c.failSubscription(watcher, acceleratorsecret.ReasonCapacity)
		}
		return true
	case acceleratorsecret.EventWatcherStatus:
		var status acceleratorsecret.SecretWatcherStatus
		if decodeErr := acceleratorsecret.DecodeResultValue(frame.Data, &status); decodeErr != nil {
			c.logProtocolFailure("decode_watcher_status_event", map[string]interface{}{"error": decodeErr.Error(), "payloadBytes": len(frame.Data)})
			return false
		}
		c.mu.Lock()
		watcher := c.subscriptions[status.WatcherSpecID]
		c.mu.Unlock()
		if watcher == nil {
			return true
		}
		if status.Status == acceleratorsecret.WatchStatusReconnecting {
			if !watcher.deliver(SecretWatchEvent{status: &status}) {
				c.failSubscription(watcher, acceleratorsecret.ReasonCapacity)
				return true
			}
			c.failSubscription(watcher, acceleratorsecret.ReasonWatchGap)
			return true
		}
		if status.Status != acceleratorsecret.WatchStatusConnected {
			c.failSubscription(watcher, acceleratorsecret.ReasonWatchGap)
			return true
		}
		if !watcher.deliver(SecretWatchEvent{status: &status}) {
			c.failSubscription(watcher, acceleratorsecret.ReasonCapacity)
		}
		return true
	case acceleratorsecret.EventWatcherError:
		var watchError acceleratorsecret.SecretWatcherError
		if decodeErr := acceleratorsecret.DecodeResultValue(frame.Data, &watchError); decodeErr != nil {
			c.logProtocolFailure("decode_watcher_error_event", map[string]interface{}{"error": decodeErr.Error(), "payloadBytes": len(frame.Data)})
			return false
		}
		c.mu.Lock()
		watcher := c.subscriptions[watchError.WatcherSpecID]
		c.mu.Unlock()
		if watcher != nil {
			if watchError.Code != acceleratorsecret.WatchErrorUnavailable && watchError.Code != acceleratorsecret.WatchErrorResourceVersionExpired && watchError.Code != acceleratorsecret.WatchErrorMalformed {
				c.failSubscription(watcher, acceleratorsecret.ReasonWatchGap)
				return true
			}
			if !watcher.deliver(SecretWatchEvent{watchError: &watchError}) {
				c.failSubscription(watcher, acceleratorsecret.ReasonCapacity)
				return true
			}
			c.failSubscription(watcher, acceleratorsecret.ReasonWatchGap)
		}
		return true
	default:
		c.logProtocolFailure("validate_watch_event_name", map[string]interface{}{"event": frame.Name})
		return false
	}
}

func (c *secretRPCClient) failSubscription(watcher *secretWatchLease, reason acceleratorsecret.SecretClientReason) {
	if c.removeSubscription(watcher) {
		if reason == acceleratorsecret.ReasonCapacity {
			watcher.terminateClearing(reason)
		} else {
			watcher.terminate(reason)
		}
		c.bestEffortUnsubscribe(watcher.subscription.WatcherSpecID)
	}
}

func (c *secretRPCClient) removeSubscription(watcher *secretWatchLease) bool {
	if watcher == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	id := watcher.subscription.WatcherSpecID
	if c.subscriptions[id] != watcher {
		return false
	}
	delete(c.subscriptions, id)
	return true
}

func (c *secretRPCClient) bestEffortUnsubscribe(id acceleratorsecret.SecretWatchSpecID) {
	if !acceleratorsecret.ValidSecretWatchSpecID(id) {
		return
	}
	c.mu.Lock()
	c.callCounter++
	callID, ok := acceleratorsecret.CallID(c.nonce, c.callCounter)
	if !ok {
		c.mu.Unlock()
		return
	}
	payload, err := acceleratorsecret.EncodeUnsubscribeSecretWatcherCall(callID, string(id))
	if err == nil {
		_ = c.session.sendApplicationFrame(payload)
	}
	c.mu.Unlock()
	clear(payload)
}

func reasonFromError(err error) acceleratorsecret.SecretClientReason {
	if failure, ok := err.(secretClientError); ok {
		return failure.reason
	}
	return acceleratorsecret.ReasonSessionUnavailable
}
