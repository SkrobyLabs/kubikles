package acceleratorprovision

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/httpstream"
)

func retryableResumeFailure(failure *connectAttemptFailure) bool {
	if failure == nil {
		return false
	}
	if failure.status == 502 || failure.status == 503 || failure.status == 504 {
		return failure.phase == attemptInfo || failure.phase == attemptPolicy || failure.phase == attemptWebSocket
	}
	if failure.peerClose {
		return failure.phase == attemptPublication
	}
	if failure.earlyClose && failure.cause == nil {
		return failure.phase == attemptTunnel || failure.phase == attemptPublication
	}
	err := failure.cause
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var protocol protocolAttemptError
	var identity identityAttemptError
	var workload workloadValidationError
	if errors.As(err, &protocol) || errors.As(err, &identity) {
		return false
	}
	if errors.As(err, &workload) {
		return retryableKubernetesCause(workload.cause)
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	if errors.Is(err, errTunnelUpgradeAttempt) || errors.Is(err, errTunnelHTTPSProxyAttempt) || errors.Is(err, errTunnelTransientAttempt) || httpstream.IsUpgradeFailure(err) || httpstream.IsHTTPSProxyError(err) || retryableKubernetesCause(err) {
		return true
	}
	return false
}

func retryableKubernetesCause(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsTimeout(err) || apierrors.IsServerTimeout(err) || apierrors.IsTooManyRequests(err) || apierrors.IsInternalError(err) || apierrors.IsServiceUnavailable(err) {
		return true
	}
	var status apierrors.APIStatus
	if errors.As(err, &status) {
		code := status.Status().Code
		return code == 500 || code == 503 || code == 504
	}
	return false
}

func resumeRetryNeedsExactRevalidation(failure *connectAttemptFailure) bool {
	if failure == nil {
		return false
	}
	if failure.earlyClose {
		return true
	}
	err := failure.cause
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed) || errors.Is(err, syscall.ECONNRESET)
}

func terminalResumeReason(failure *connectAttemptFailure) ResumeReason {
	if failure == nil {
		return ResumeTransportUnavailable
	}
	if failure.status == 401 || failure.status == 403 {
		return ResumeAuthenticationFailed
	}
	var identity identityAttemptError
	var protocol protocolAttemptError
	var mismatch buildVersionMismatchAttemptError
	var workload workloadValidationError
	if errors.As(failure.cause, &mismatch) {
		return ResumeVersionMismatch
	}
	if errors.As(failure.cause, &identity) {
		return ResumeIdentityMismatch
	}
	if errors.As(failure.cause, &protocol) {
		return ResumeProtocolFailed
	}
	if errors.As(failure.cause, &workload) || failure.connectReason == InvalidWorkload || failure.connectReason == WorkloadChanged || failure.connectReason == WorkloadUnavailable {
		return ResumeWorkloadTerminalOrChanged
	}
	return ResumeTransportUnavailable
}
