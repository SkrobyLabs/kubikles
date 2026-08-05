package acceleratorprovision

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"kubikles/pkg/agent"
)

type ResumeReason string

const (
	ResumeInvalid                   ResumeReason = "invalid_resume"
	ResumeSessionIneligible         ResumeReason = "session_not_resume_eligible"
	ResumeSuperseded                ResumeReason = "resume_superseded"
	ResumeWorkloadTerminalOrChanged ResumeReason = "workload_terminal_or_changed"
	ResumeAuthenticationFailed      ResumeReason = "authentication_failed"
	ResumeIdentityMismatch          ResumeReason = "identity_mismatch"
	ResumeProtocolFailed            ResumeReason = "protocol_failed"
	ResumeVersionMismatch           ResumeReason = "version_mismatch"
	ResumeTransportUnavailable      ResumeReason = "transport_unavailable"
	ResumeCancelled                 ResumeReason = "cancelled"
	ResumeGraceExpired              ResumeReason = "grace_expired"
	ResumeExplicitlyClosed          ResumeReason = "explicitly_closed"
	ResumeWorkloadDisposing         ResumeReason = "workload_disposing"
)

type ResumeRequest struct {
	Prior    *ConnectedSession    `json:"prior,omitempty"`
	Workload *ProvisionedWorkload `json:"workload,omitempty"`
}

func (ResumeRequest) String() string { return "<accelerator resume request>" }
func (ResumeRequest) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator resume request>")
}
func (ResumeRequest) MarshalJSON() ([]byte, error) {
	return []byte(`"<accelerator resume request>"`), nil
}

type ResumeResult struct {
	Availability Availability      `json:"availability"`
	Reason       ResumeReason      `json:"reason,omitempty"`
	Session      *ConnectedSession `json:"session,omitempty"`
}

func (ResumeResult) String() string { return "<accelerator resume result>" }
func (ResumeResult) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "<accelerator resume result>")
}
func (r ResumeResult) MarshalJSON() ([]byte, error) {
	type safe struct {
		Availability Availability      `json:"availability"`
		Reason       ResumeReason      `json:"reason,omitempty"`
		Session      *ConnectedSession `json:"session,omitempty"`
	}
	return json.Marshal(safe{Availability: r.Availability, Reason: r.Reason, Session: r.Session})
}

type resumeAttempt func(context.Context, connectorLease, connectionExpectation) (*ConnectedSession, *connectAttemptFailure)

type Reconnector struct {
	connector    *Connector
	attempt      resumeAttempt
	afterPublish func(*ConnectedSession)
}

func NewReconnector(buildVersion string) *Reconnector {
	return &Reconnector{connector: NewConnector(buildVersion)}
}

func unavailableResume(reason ResumeReason) ResumeResult {
	return ResumeResult{Availability: Unavailable, Reason: reason}
}

func (r *Reconnector) Resume(ctx context.Context, request ResumeRequest) (result ResumeResult) {
	return r.resume(ctx, request, nil)
}

func (r *Reconnector) ResumeWithVersionPolicy(ctx context.Context, request ResumeRequest, expectedVersion string, allowMismatch bool) ResumeResult {
	configured := r.withVersionPolicy(expectedVersion, allowMismatch)
	if configured == nil {
		return unavailableResume(ResumeInvalid)
	}
	return configured.resume(ctx, request, nil)
}

func (r *Reconnector) resumeIdle(ctx context.Context, request ResumeRequest, idle *coordinatorIdleToken) ResumeResult {
	if idle == nil {
		return unavailableResume(ResumeSessionIneligible)
	}
	return r.resume(ctx, request, idle)
}

func (r *Reconnector) resumeIdleWithVersionPolicy(ctx context.Context, request ResumeRequest, idle *coordinatorIdleToken, expectedVersion string, allowMismatch bool) ResumeResult {
	configured := r.withVersionPolicy(expectedVersion, allowMismatch)
	if configured == nil {
		return unavailableResume(ResumeInvalid)
	}
	return configured.resumeIdle(ctx, request, idle)
}

func (r *Reconnector) withVersionPolicy(expectedVersion string, allowMismatch bool) *Reconnector {
	if r == nil || r.connector == nil || expectedVersion == "" {
		return nil
	}
	configured := *r
	connector := *r.connector
	connector.buildVersion = expectedVersion
	connector.allowVersionMismatch = allowMismatch
	configured.connector = &connector
	return &configured
}

func (r *Reconnector) resume(ctx context.Context, request ResumeRequest, idle *coordinatorIdleToken) (result ResumeResult) {
	if r == nil || r.connector == nil || ctx == nil || r.connector.buildVersion == "" || r.connector.startTunnel == nil || (r.connector.validate == nil && r.connector.validateDetailed == nil) || request.Prior == nil || !validWorkloadHandle(request.Workload, r.connector.buildVersion) {
		return unavailableResume(ResumeInvalid)
	}
	opLifecycle, finishLifecycle, lifecycleReason := request.Workload.beginLifecycleOperation(ctx)
	if lifecycleReason != "" {
		if lifecycleReason == InvalidWorkload {
			return unavailableResume(ResumeInvalid)
		}
		return unavailableResume(ResumeWorkloadDisposing)
	}
	defer finishLifecycle()
	defer func() {
		if request.Workload.isDisposing() {
			if result.Session != nil {
				_ = result.Session.Close(context.Background())
			}
			result = unavailableResume(ResumeWorkloadDisposing)
		}
	}()
	record, resumeStop, reason := request.Prior.claimResumeWithIdle(request.Workload, idle)
	if reason != "" {
		return unavailableResume(reason)
	}
	clock := request.Prior.clock
	if clock == nil {
		return unavailableResume(ResumeInvalid)
	}
	deadline := record.at.Add(agent.AcceleratorIdleReconnectGrace)
	op, cancel := context.WithCancel(opLifecycle)
	watchDone := make(chan struct{})
	go func() {
		select {
		case <-resumeStop:
			cancel()
		case <-op.Done():
		}
		close(watchDone)
	}()
	defer func() {
		cancel()
		<-watchDone
	}()

	connector := *r.connector
	connector.clock = clock
	attempt := r.attempt
	if attempt == nil {
		attempt = connector.connectExactAttempt
	}
	var fenceMu sync.Mutex
	highestGeneration := record.generation
	var currentEpoch uint64
	epochActive := false
	backoff := time.Second
	for epoch := uint64(1); ; epoch++ {
		if stopReason := resumeStopReason(ctx, resumeStop, clock, deadline); stopReason != "" {
			return unavailableResume(stopReason)
		}
		remaining := deadline.Sub(clock.Now())
		budget := connectorTimeout
		if remaining < budget {
			budget = remaining
		}
		attemptCtx, attemptCancel := context.WithTimeout(op, budget)
		lease, ok := request.Workload.resumeConnectorLease(record.workloadNonce)
		if !ok {
			attemptCancel()
			return unavailableResume(ResumeWorkloadTerminalOrChanged)
		}
		fenceMu.Lock()
		currentEpoch = epoch
		epochActive = true
		generationFloor := highestGeneration
		fenceMu.Unlock()
		session, failure := attempt(attemptCtx, lease, connectionExpectation{
			kind: connectionResume, instanceID: record.instanceID, sessionID: record.sessionID,
			generationFloor: generationFloor,
			accepted: func(generation int) {
				fenceMu.Lock()
				if epochActive && currentEpoch == epoch && generation > highestGeneration {
					highestGeneration = generation
				}
				fenceMu.Unlock()
			},
		})
		fenceMu.Lock()
		if currentEpoch == epoch {
			epochActive = false
		}
		fenceMu.Unlock()
		retryable := retryableResumeFailure(failure)
		if retryable && resumeRetryNeedsExactRevalidation(failure) && attemptCtx.Err() == nil {
			if validation := connector.validateExact(sealedContext{attemptCtx}, lease.receipt.workload(), lease.snapshot); validation.reason != "" {
				failure = workloadAttemptFailure(attemptFinalValidate, validation)
				retryable = retryableResumeFailure(failure)
			}
		}
		attemptCancel()
		lease.release()
		if stopReason := resumeStopReason(ctx, resumeStop, clock, deadline); stopReason != "" {
			if session != nil {
				_ = session.Close(context.Background())
			}
			return unavailableResume(stopReason)
		}
		if session != nil && failure == nil {
			if !request.Workload.publishCurrentSession(session) {
				_ = session.Close(context.Background())
				return unavailableResume(ResumeWorkloadDisposing)
			}
			if r.afterPublish != nil {
				r.afterPublish(session)
			}
			return ResumeResult{Availability: Available, Session: session}
		}
		if !retryable {
			return unavailableResume(terminalResumeReason(failure))
		}
		remaining = deadline.Sub(clock.Now())
		if remaining <= 0 {
			return unavailableResume(ResumeGraceExpired)
		}
		sleep := backoff
		if sleep > 5*time.Second {
			sleep = 5 * time.Second
		}
		if sleep > remaining {
			sleep = remaining
		}
		if err := clock.Sleep(op, sleep); err != nil {
			stopReason := resumeStopReason(ctx, resumeStop, clock, deadline)
			if stopReason == "" {
				stopReason = ResumeCancelled
			}
			return unavailableResume(stopReason)
		}
		if backoff < 4*time.Second {
			backoff *= 2
		} else {
			backoff = 5 * time.Second
		}
	}
}

func resumeStopReason(caller context.Context, resumeStop <-chan struct{}, clock resumeClock, deadline time.Time) ResumeReason {
	if caller != nil && caller.Err() != nil {
		return ResumeCancelled
	}
	select {
	case <-resumeStop:
		return ResumeExplicitlyClosed
	default:
	}
	if !clock.Now().Before(deadline) {
		return ResumeGraceExpired
	}
	return ""
}
