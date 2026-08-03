//go:build !accelerator

package main

import (
	"context"
	"sync"

	"kubikles/pkg/acceleratorprovision"
	"kubikles/pkg/acceleratorsecret"
)

type secretWatchRouteOwner struct {
	epoch    uint64
	token    SecretReadSourceToken
	identity secretRouterSessionIdentity
	client   acceleratorprovision.SecretRPCClient
	lease    acceleratorprovision.SecretWatchLease
	stop     chan struct{}
	stopOnce sync.Once
}

func (r *integratedSecretRouter) SubscribeSecretWatcher(ctx context.Context, token SecretReadSourceToken, namespace string, excludeHelmReleases bool) (acceleratorsecret.SecretWatchSubscription, error) {
	op, err := r.captureOperation(ctx, token)
	if err != nil {
		return acceleratorsecret.SecretWatchSubscription{}, err
	}
	defer r.finishOperation(op)
	subscription, lease, remoteErr := op.client.SubscribeSecretWatcher(op.ctx, namespace, excludeHelmReleases)
	if remoteErr != nil || lease == nil {
		terminal := lease == nil && remoteErr == nil
		if (terminal || secretFailurePolicy(remoteErr, op.ctx) != secretFailureReturn) && r.operationCurrent(op) {
			r.retireCurrent(op.identity)
		}
		return acceleratorsecret.SecretWatchSubscription{}, ErrIntegratedSecretReadsUnavailable
	}
	owner := &secretWatchRouteOwner{epoch: op.epoch, token: token, identity: op.identity, client: op.client, lease: lease, stop: make(chan struct{})}
	r.mu.Lock()
	current := !r.closed && !r.quiesced && r.routeEpoch == op.epoch && r.token == token && r.client == op.client && op.ctx.Err() == nil
	if current {
		if _, duplicate := r.watches[subscription.WatcherSpecID]; duplicate {
			current = false
		} else {
			r.watches[subscription.WatcherSpecID] = owner
			r.monitors.Add(1)
		}
	}
	r.mu.Unlock()
	if !current {
		lease.Close(context.Background())
		return acceleratorsecret.SecretWatchSubscription{}, ErrIntegratedSecretReadsUnavailable
	}
	go r.forwardWatch(subscription.WatcherSpecID, owner)
	return subscription, nil
}

func (r *integratedSecretRouter) forwardWatch(id acceleratorsecret.SecretWatchSpecID, owner *secretWatchRouteOwner) {
	defer r.monitors.Done()
	for {
		select {
		case <-owner.stop:
			return
		case event, open := <-owner.lease.Events():
			if !open {
				r.watchEnded(id, owner)
				return
			}
			if resource, ok := event.Resource(); ok {
				if resource.WatcherSpecID == id {
					r.publishWatchEvent(id, owner, func() {
						if r.deps.resource == nil {
							return
						}
						r.deps.resource(integratedSecretResourceSignal{
							SourceToken: owner.token, Type: resource.Type, ResourceType: resource.ResourceType,
							Namespace: resource.Namespace, WatcherSpecID: resource.WatcherSpecID, Resource: resource.Resource,
						})
					})
				}
				continue
			}
			if status, ok := event.Status(); ok {
				if status.WatcherSpecID == id {
					r.publishWatchEvent(id, owner, func() {
						if r.deps.status != nil {
							r.deps.status(integratedSecretStatusSignal{SourceToken: owner.token, WatcherSpecID: status.WatcherSpecID, Status: status.Status})
						}
					})
				}
				continue
			}
			if watchError, ok := event.Error(); ok {
				if watchError.WatcherSpecID == id {
					r.publishWatchEvent(id, owner, func() {
						if r.deps.watchError != nil {
							r.deps.watchError(integratedSecretErrorSignal{SourceToken: owner.token, WatcherSpecID: watchError.WatcherSpecID, Code: watchError.Code, Recoverable: watchError.Recoverable})
						}
					})
				}
			}
		case <-owner.lease.Done():
			r.watchEnded(id, owner)
			return
		}
	}
}

func (r *integratedSecretRouter) publishWatchEvent(id acceleratorsecret.SecretWatchSpecID, owner *secretWatchRouteOwner, emit func()) {
	if emit == nil {
		return
	}
	r.serializeEvent(func() {
		// Revalidate at the ordered emission point. An event already emitting
		// finishes before unavailable; one queued behind a detach is dropped.
		if r.watchCurrent(id, owner) {
			emit()
		}
	})
}

func (r *integratedSecretRouter) watchCurrent(id acceleratorsecret.SecretWatchSpecID, owner *secretWatchRouteOwner) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.closed && !r.quiesced && r.watches[id] == owner && r.routeEpoch == owner.epoch && r.token == owner.token && r.client == owner.client
}

func (r *integratedSecretRouter) watchEnded(id acceleratorsecret.SecretWatchSpecID, owner *secretWatchRouteOwner) {
	r.mu.Lock()
	current := r.watches[id] == owner && r.routeEpoch == owner.epoch && r.token == owner.token && r.client == owner.client
	if current {
		delete(r.watches, id)
	}
	r.mu.Unlock()
	if current {
		r.retireCurrent(owner.identity)
	}
}

func (r *integratedSecretRouter) UnsubscribeSecretWatcher(ctx context.Context, token SecretReadSourceToken, id acceleratorsecret.SecretWatchSpecID) error {
	r.mu.Lock()
	if r.closed || r.quiesced || token == "" || token != r.token || r.client == nil {
		r.mu.Unlock()
		return ErrIntegratedSecretReadsUnavailable
	}
	owner := r.watches[id]
	if owner == nil || owner.token != token || owner.epoch != r.routeEpoch {
		r.mu.Unlock()
		return nil
	}
	delete(r.watches, id)
	owner.stopOnce.Do(func() { close(owner.stop) })
	r.mu.Unlock()
	if err := owner.client.UnsubscribeSecretWatcher(ctx, id); err != nil {
		if secretFailurePolicy(err, ctx) == secretFailureRetire {
			r.retireCurrent(owner.identity)
		}
		return ErrIntegratedSecretReadsUnavailable
	}
	return nil
}
