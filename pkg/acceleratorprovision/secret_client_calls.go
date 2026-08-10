package acceleratorprovision

import (
	"context"
	"fmt"

	"kubikles/pkg/acceleratorsecret"
	"kubikles/pkg/helm"
	"kubikles/pkg/k8s"
)

type secretCallEncoder func(string) ([]byte, error)

func (c *secretRPCClient) ListSecretsMetadata(ctx context.Context, requestID, namespace string, excludeHelmReleases bool) ([]k8s.SecretListItem, error) {
	pending, err := c.startListCall(acceleratorsecret.OperationListSecretsMetadata, requestID, func(id, remoteID string) ([]byte, error) {
		return acceleratorsecret.EncodeListSecretsMetadataCall(id, remoteID, namespace, excludeHelmReleases)
	})
	if err != nil {
		return nil, err
	}
	frame, err := c.waitCall(ctx, pending)
	if err != nil {
		return nil, err
	}
	defer clear(frame.Result)
	var items []acceleratorsecret.SecretListItem
	if decodeErr := acceleratorsecret.DecodeResultValue(frame.Result, &items); decodeErr != nil {
		c.logProtocolFailure("decode_list_secrets_result", map[string]interface{}{"error": decodeErr.Error(), "payloadBytes": len(frame.Result)})
		c.terminate(acceleratorsecret.ReasonProtocol)
		return nil, newSecretClientError(acceleratorsecret.ReasonProtocol)
	}
	return acceleratorsecret.KubernetesSecretListItems(items), nil
}

func (c *secretRPCClient) ListHelmReleaseMetadata(ctx context.Context, requestID, namespace string) ([]helm.Release, error) {
	pending, err := c.startListCall(acceleratorsecret.OperationListHelmReleaseMetadata, requestID, func(id, remoteID string) ([]byte, error) {
		return acceleratorsecret.EncodeListHelmReleaseMetadataCall(id, remoteID, namespace)
	})
	if err != nil {
		return nil, err
	}
	frame, err := c.waitCall(ctx, pending)
	if err != nil {
		return nil, err
	}
	defer clear(frame.Result)
	var projected []acceleratorsecret.HelmReleaseMetadata
	if decodeErr := acceleratorsecret.DecodeResultValue(frame.Result, &projected); decodeErr != nil {
		c.logProtocolFailure("decode_list_helm_release_metadata_result", map[string]interface{}{"error": decodeErr.Error(), "payloadBytes": len(frame.Result)})
		c.terminate(acceleratorsecret.ReasonProtocol)
		return nil, newSecretClientError(acceleratorsecret.ReasonProtocol)
	}
	releases := make([]helm.Release, len(projected))
	for index, release := range projected {
		releases[index] = helm.Release{
			Name: release.Name, Namespace: release.Namespace, Revision: release.Revision,
			Status: release.Status, Chart: release.Chart, ChartVersion: release.ChartVersion,
			AppVersion: release.AppVersion, Updated: release.Updated, Description: release.Description,
		}
	}
	return releases, nil
}

func (c *secretRPCClient) GetSecretData(ctx context.Context, namespace, name string) ([]k8s.DataEntry, error) {
	pending, err := c.startCall(acceleratorsecret.OperationGetSecretData, func(id string) ([]byte, error) {
		return acceleratorsecret.EncodeGetSecretDataCall(id, namespace, name)
	})
	if err != nil {
		return nil, err
	}
	frame, err := c.waitCall(ctx, pending)
	if err != nil {
		return nil, err
	}
	defer clear(frame.Result)
	var entries []k8s.DataEntry
	if decodeErr := acceleratorsecret.DecodeResultValue(frame.Result, &entries); decodeErr != nil {
		c.logProtocolFailure("decode_get_secret_data_result", map[string]interface{}{"error": decodeErr.Error(), "payloadBytes": len(frame.Result)})
		c.terminate(acceleratorsecret.ReasonProtocol)
		return nil, newSecretClientError(acceleratorsecret.ReasonProtocol)
	}
	return append([]k8s.DataEntry(nil), entries...), nil
}

func (c *secretRPCClient) GetSecretYaml(ctx context.Context, namespace, name string) (string, error) {
	pending, err := c.startCall(acceleratorsecret.OperationGetSecretYAML, func(id string) ([]byte, error) {
		return acceleratorsecret.EncodeGetSecretYAMLCall(id, namespace, name)
	})
	if err != nil {
		return "", err
	}
	frame, err := c.waitCall(ctx, pending)
	if err != nil {
		return "", err
	}
	defer clear(frame.Result)
	var value string
	if decodeErr := acceleratorsecret.DecodeResultValue(frame.Result, &value); decodeErr != nil {
		c.logProtocolFailure("decode_get_secret_yaml_result", map[string]interface{}{"error": decodeErr.Error(), "payloadBytes": len(frame.Result)})
		c.terminate(acceleratorsecret.ReasonProtocol)
		return "", newSecretClientError(acceleratorsecret.ReasonProtocol)
	}
	return value, nil
}

func (c *secretRPCClient) CancelListRequest(ctx context.Context, requestID string) (bool, error) {
	remoteID, found := c.cancelLocalList(requestID, acceleratorsecret.ReasonCanceled)
	if !found {
		return false, nil
	}
	pending, err := c.startCall(acceleratorsecret.OperationCancelListRequest, func(id string) ([]byte, error) {
		return acceleratorsecret.EncodeCancelListRequestCall(id, remoteID)
	})
	if err != nil {
		return false, err
	}
	frame, err := c.waitCall(ctx, pending)
	if err != nil {
		return false, err
	}
	var canceled bool
	if decodeErr := acceleratorsecret.DecodeResultValue(frame.Result, &canceled); decodeErr != nil {
		c.logProtocolFailure("decode_cancel_list_result", map[string]interface{}{"error": decodeErr.Error(), "payloadBytes": len(frame.Result)})
		clear(frame.Result)
		c.terminate(acceleratorsecret.ReasonProtocol)
		return false, newSecretClientError(acceleratorsecret.ReasonProtocol)
	}
	clear(frame.Result)
	return canceled, nil
}

func (c *secretRPCClient) startCall(operation acceleratorsecret.Operation, encode secretCallEncoder) (*secretPendingCall, error) {
	c.mu.Lock()
	if !c.accepting {
		reason := c.reason
		c.mu.Unlock()
		return nil, newSecretClientError(reason)
	}
	if len(c.calls) >= acceleratorsecret.MaxConcurrentCalls {
		c.mu.Unlock()
		return nil, newSecretClientError(acceleratorsecret.ReasonCapacity)
	}
	c.callCounter++
	id, ok := acceleratorsecret.CallID(c.nonce, c.callCounter)
	if !ok {
		c.mu.Unlock()
		return nil, newSecretClientError(acceleratorsecret.ReasonProtocol)
	}
	pending := &secretPendingCall{id: id, operation: operation, result: make(chan secretCallResult, 1)}
	c.calls[id] = pending
	payload, err := encode(id)
	sendReason := acceleratorsecret.ReasonProtocol
	if err == nil {
		sendReason = c.session.sendApplicationFrame(payload)
	}
	if err != nil || sendReason != "" {
		delete(c.calls, id)
		c.mu.Unlock()
		clear(payload)
		return nil, newSecretClientError(sendReason)
	}
	c.mu.Unlock()
	clear(payload)
	return pending, nil
}

func (c *secretRPCClient) startListCall(operation acceleratorsecret.Operation, callerID string, encode func(string, string) ([]byte, error)) (*secretPendingCall, error) {
	c.mu.Lock()
	if !c.accepting {
		reason := c.reason
		c.mu.Unlock()
		return nil, newSecretClientError(reason)
	}
	if len(c.calls) >= acceleratorsecret.MaxConcurrentCalls {
		c.mu.Unlock()
		return nil, newSecretClientError(acceleratorsecret.ReasonCapacity)
	}
	if _, duplicate := c.lists[callerID]; duplicate {
		c.mu.Unlock()
		return nil, newSecretClientError(acceleratorsecret.ReasonCapacity)
	}
	c.callCounter++
	c.listCounter++
	id, idOK := acceleratorsecret.CallID(c.nonce, c.callCounter)
	remoteID := fmt.Sprintf("l.%s.%016x", c.nonce, c.listCounter)
	if !idOK {
		c.mu.Unlock()
		return nil, newSecretClientError(acceleratorsecret.ReasonProtocol)
	}
	pending := &secretPendingCall{id: id, operation: operation, result: make(chan secretCallResult, 1), callerListID: callerID, remoteListID: remoteID}
	c.calls[id] = pending
	c.lists[callerID] = pending
	payload, err := encode(id, remoteID)
	sendReason := acceleratorsecret.ReasonProtocol
	if err == nil {
		sendReason = c.session.sendApplicationFrame(payload)
	}
	if err != nil || sendReason != "" {
		delete(c.calls, id)
		delete(c.lists, callerID)
		c.mu.Unlock()
		clear(payload)
		return nil, newSecretClientError(sendReason)
	}
	c.mu.Unlock()
	clear(payload)
	return pending, nil
}

func (c *secretRPCClient) waitCall(ctx context.Context, pending *secretPendingCall) (acceleratorsecret.ResultFrame, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	callCtx, cancel := context.WithTimeout(ctx, acceleratorsecret.OperationTimeout(pending.operation))
	defer cancel()
	select {
	case result := <-pending.result:
		return resultFrame(result.frame)
	case <-callCtx.Done():
		if c.removeCall(pending) {
			if pending.remoteListID != "" {
				c.bestEffortCancel(pending.remoteListID)
			}
			reason := acceleratorsecret.ReasonCanceled
			if callCtx.Err() == context.DeadlineExceeded {
				reason = acceleratorsecret.ReasonDeadline
			}
			return acceleratorsecret.ResultFrame{}, newSecretClientError(reason)
		}
		result := <-pending.result
		return resultFrame(result.frame)
	case <-c.done:
		select {
		case result := <-pending.result:
			return resultFrame(result.frame)
		default:
			return acceleratorsecret.ResultFrame{}, newSecretClientError(c.Reason())
		}
	}
}

func resultFrame(frame acceleratorsecret.ResultFrame) (acceleratorsecret.ResultFrame, error) {
	if frame.Status == acceleratorsecret.ResultStatusError {
		return acceleratorsecret.ResultFrame{}, newSecretClientError(frame.Reason)
	}
	return frame, nil
}

func (c *secretRPCClient) removeCall(pending *secretPendingCall) bool {
	if c.beforeRemoveCall != nil {
		c.beforeRemoveCall()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.calls[pending.id] != pending {
		return false
	}
	delete(c.calls, pending.id)
	if pending.remoteListID != "" && c.lists[pending.callerListID] == pending {
		delete(c.lists, pending.callerListID)
	}
	return true
}

func (c *secretRPCClient) cancelLocalList(callerID string, reason acceleratorsecret.SecretClientReason) (string, bool) {
	c.mu.Lock()
	pending := c.lists[callerID]
	if pending == nil || c.calls[pending.id] != pending {
		c.mu.Unlock()
		return "", false
	}
	delete(c.lists, callerID)
	delete(c.calls, pending.id)
	remoteID := pending.remoteListID
	c.mu.Unlock()
	pending.result <- secretCallResult{frame: acceleratorsecret.ResultFrame{Status: acceleratorsecret.ResultStatusError, Reason: reason}}
	return remoteID, true
}

func (c *secretRPCClient) bestEffortCancel(remoteID string) {
	if remoteID == "" {
		return
	}
	c.mu.Lock()
	c.callCounter++
	id, ok := acceleratorsecret.CallID(c.nonce, c.callCounter)
	if !ok {
		c.mu.Unlock()
		return
	}
	payload, err := acceleratorsecret.EncodeCancelListRequestCall(id, remoteID)
	if err == nil {
		_ = c.session.sendApplicationFrame(payload)
	}
	c.mu.Unlock()
	clear(payload)
}
