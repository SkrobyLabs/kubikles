//go:build !accelerator

package main

import (
	"context"
	"errors"

	"kubikles/pkg/acceleratorprovision"
	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/helm"
	"kubikles/pkg/k8s"
)

type secretListRouteKind uint8

const (
	secretListRouteRemote secretListRouteKind = iota + 1
	secretListRouteDirect
)

type secretListRouteOwner struct {
	opID   uint64
	epoch  uint64
	token  SecretReadSourceToken
	kind   secretListRouteKind
	cancel context.CancelFunc
	client acceleratorprovision.SecretRPCClient
}

type integratedSecretOperation struct {
	id       uint64
	epoch    uint64
	token    SecretReadSourceToken
	ctx      context.Context
	cancel   context.CancelFunc
	client   acceleratorprovision.SecretRPCClient
	identity secretRouterSessionIdentity
}

func (r *integratedSecretRouter) captureOperation(ctx context.Context, token SecretReadSourceToken) (integratedSecretOperation, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.quiesced || token == "" || token != r.token || r.client == nil {
		return integratedSecretOperation{}, ErrIntegratedSecretReadsUnavailable
	}
	opCtx, cancel := context.WithCancel(ctx)
	r.opCounter++
	op := integratedSecretOperation{id: r.opCounter, epoch: r.routeEpoch, token: token, ctx: opCtx, cancel: cancel, client: r.client, identity: r.sessionIdentity}
	r.operations[op.id] = cancel
	return op, nil
}

func (r *integratedSecretRouter) finishOperation(op integratedSecretOperation) {
	r.mu.Lock()
	if cancel := r.operations[op.id]; cancel != nil {
		delete(r.operations, op.id)
	}
	r.mu.Unlock()
	op.cancel()
}

func (r *integratedSecretRouter) operationCurrent(op integratedSecretOperation) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.closed && !r.quiesced && op.ctx.Err() == nil && r.routeEpoch == op.epoch && r.token == op.token && r.client == op.client && r.sessionIdentity == op.identity
}

type secretFailureAction uint8

const (
	secretFailureReturn secretFailureAction = iota
	secretFailureFallback
	secretFailureRetire
)

func secretFailurePolicy(err error, ctx context.Context) secretFailureAction {
	if err == nil || (ctx != nil && ctx.Err() != nil) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return secretFailureReturn
	}
	reason, ok := acceleratorprovision.SecretClientReasonOf(err)
	if !ok {
		return secretFailureRetire
	}
	switch reason {
	case acceleratorsecret.ReasonCapacity, acceleratorsecret.ReasonForbidden, acceleratorsecret.ReasonRemoteUnavailable:
		return secretFailureFallback
	case acceleratorsecret.ReasonCanceled, acceleratorsecret.ReasonDeadline:
		return secretFailureReturn
	case acceleratorsecret.ReasonProtocol, acceleratorsecret.ReasonSessionUnavailable, acceleratorsecret.ReasonWatchGap, acceleratorsecret.ReasonClosed:
		return secretFailureRetire
	default:
		return secretFailureRetire
	}
}

func (r *integratedSecretRouter) fixedOperationError(op integratedSecretOperation, err error) error {
	if secretFailurePolicy(err, op.ctx) == secretFailureRetire && r.operationCurrent(op) {
		r.retireCurrent(op.identity)
	}
	return ErrIntegratedSecretReadsUnavailable
}

func (r *integratedSecretRouter) ListSecretsMetadata(ctx context.Context, token SecretReadSourceToken, requestID, namespace string, excludeHelmReleases bool) ([]k8s.SecretListItem, error) {
	op, err := r.captureOperation(ctx, token)
	if err != nil {
		return nil, err
	}
	owner := &secretListRouteOwner{opID: op.id, epoch: op.epoch, token: token, kind: secretListRouteRemote, cancel: op.cancel, client: op.client}
	r.mu.Lock()
	if _, duplicate := r.lists[requestID]; duplicate || r.routeEpoch != op.epoch || r.token != token {
		r.mu.Unlock()
		r.finishOperation(op)
		return nil, ErrIntegratedSecretReadsUnavailable
	}
	r.lists[requestID] = owner
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		if r.lists[requestID] == owner {
			delete(r.lists, requestID)
		}
		r.mu.Unlock()
		r.finishOperation(op)
	}()

	rows, remoteErr := op.client.ListSecretsMetadata(op.ctx, requestID, namespace, excludeHelmReleases)
	if remoteErr == nil {
		if !r.operationCurrent(op) {
			return nil, ErrIntegratedSecretReadsUnavailable
		}
		return rows, nil
	}
	if secretFailurePolicy(remoteErr, op.ctx) != secretFailureFallback || !r.operationCurrent(op) || r.deps.directList == nil {
		return nil, r.fixedOperationError(op, remoteErr)
	}
	r.mu.Lock()
	current := r.lists[requestID] == owner && r.routeEpoch == op.epoch && r.token == token && op.ctx.Err() == nil
	if current {
		owner.kind = secretListRouteDirect
	}
	r.mu.Unlock()
	if !current {
		return nil, ErrIntegratedSecretReadsUnavailable
	}
	rows, directErr := r.deps.directList(op.ctx, requestID, namespace, excludeHelmReleases)
	if directErr != nil || !r.operationCurrent(op) {
		return nil, ErrIntegratedSecretReadsUnavailable
	}
	return rows, nil
}

func (r *integratedSecretRouter) ListHelmReleaseMetadata(ctx context.Context, token SecretReadSourceToken, requestID, namespace string) ([]helm.Release, error) {
	op, err := r.captureOperation(ctx, token)
	if err != nil {
		return nil, err
	}
	owner := &secretListRouteOwner{opID: op.id, epoch: op.epoch, token: token, kind: secretListRouteRemote, cancel: op.cancel, client: op.client}
	r.mu.Lock()
	if _, duplicate := r.lists[requestID]; duplicate || r.routeEpoch != op.epoch || r.token != token {
		r.mu.Unlock()
		r.finishOperation(op)
		return nil, ErrIntegratedSecretReadsUnavailable
	}
	r.lists[requestID] = owner
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		if r.lists[requestID] == owner {
			delete(r.lists, requestID)
		}
		r.mu.Unlock()
		r.finishOperation(op)
	}()

	remote, supported := op.client.(acceleratorprovision.HelmReleaseRPCClient)
	if supported {
		releases, remoteErr := remote.ListHelmReleaseMetadata(op.ctx, requestID, namespace)
		if remoteErr == nil {
			if !r.operationCurrent(op) {
				return nil, ErrIntegratedSecretReadsUnavailable
			}
			return releases, nil
		}
		if secretFailurePolicy(remoteErr, op.ctx) != secretFailureFallback || !r.operationCurrent(op) || r.deps.directHelmReleaseList == nil {
			return nil, r.fixedOperationError(op, remoteErr)
		}
	} else if r.deps.directHelmReleaseList == nil || !r.operationCurrent(op) {
		return nil, ErrIntegratedSecretReadsUnavailable
	}

	r.mu.Lock()
	current := r.lists[requestID] == owner && r.routeEpoch == op.epoch && r.token == token && op.ctx.Err() == nil
	if current {
		owner.kind = secretListRouteDirect
	}
	r.mu.Unlock()
	if !current {
		return nil, ErrIntegratedSecretReadsUnavailable
	}
	releases, directErr := r.deps.directHelmReleaseList(op.ctx, requestID, namespace)
	if directErr != nil || !r.operationCurrent(op) {
		return nil, ErrIntegratedSecretReadsUnavailable
	}
	return releases, nil
}

func (r *integratedSecretRouter) GetSecretData(ctx context.Context, token SecretReadSourceToken, namespace, name string) ([]k8s.DataEntry, error) {
	op, err := r.captureOperation(ctx, token)
	if err != nil {
		return nil, err
	}
	defer r.finishOperation(op)
	data, remoteErr := op.client.GetSecretData(op.ctx, namespace, name)
	if remoteErr == nil {
		if !r.operationCurrent(op) {
			return nil, ErrIntegratedSecretReadsUnavailable
		}
		return data, nil
	}
	if secretFailurePolicy(remoteErr, op.ctx) == secretFailureFallback && r.operationCurrent(op) && r.deps.directData != nil {
		data, directErr := r.deps.directData(namespace, name)
		if directErr == nil && r.operationCurrent(op) {
			return data, nil
		}
		return nil, ErrIntegratedSecretReadsUnavailable
	}
	return nil, r.fixedOperationError(op, remoteErr)
}

func (r *integratedSecretRouter) GetSecretYaml(ctx context.Context, token SecretReadSourceToken, namespace, name string) (string, error) {
	op, err := r.captureOperation(ctx, token)
	if err != nil {
		return "", err
	}
	defer r.finishOperation(op)
	value, remoteErr := op.client.GetSecretYaml(op.ctx, namespace, name)
	if remoteErr == nil {
		if !r.operationCurrent(op) {
			return "", ErrIntegratedSecretReadsUnavailable
		}
		return value, nil
	}
	if secretFailurePolicy(remoteErr, op.ctx) == secretFailureFallback && r.operationCurrent(op) && r.deps.directYAML != nil {
		value, directErr := r.deps.directYAML(namespace, name)
		if directErr == nil && r.operationCurrent(op) {
			return value, nil
		}
		return "", ErrIntegratedSecretReadsUnavailable
	}
	return "", r.fixedOperationError(op, remoteErr)
}

func (r *integratedSecretRouter) CancelListRequest(ctx context.Context, token SecretReadSourceToken, requestID string) (bool, error) {
	r.mu.Lock()
	if r.closed || r.quiesced || token == "" || token != r.token || r.client == nil {
		r.mu.Unlock()
		return false, ErrIntegratedSecretReadsUnavailable
	}
	owner := r.lists[requestID]
	if owner == nil || owner.token != token || owner.epoch != r.routeEpoch {
		r.mu.Unlock()
		return false, nil
	}
	delete(r.lists, requestID)
	identity := r.sessionIdentity
	r.mu.Unlock()
	if owner.kind == secretListRouteDirect {
		owner.cancel()
		return true, nil
	}
	canceled, err := owner.client.CancelListRequest(ctx, requestID)
	owner.cancel()
	if err != nil {
		if secretFailurePolicy(err, ctx) == secretFailureRetire {
			r.retireCurrent(identity)
		}
		return false, ErrIntegratedSecretReadsUnavailable
	}
	return canceled, nil
}
