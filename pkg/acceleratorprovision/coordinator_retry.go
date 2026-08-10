package acceleratorprovision

import "kubikles/pkg/acceleratorrelease"

func classifyReleaseFailure(reason acceleratorrelease.UnavailableReason) failureClass {
	return failureAuthoritative
}

func classifyProvisionFailure(reason UnavailableReason) failureClass {
	switch reason {
	case ChartPullFailed, InstallFailed, ImagePullFailed, TimedOut:
		return failureTemporary
	case Cancelled, ContextChanged:
		return failureCancelled
	default:
		return failureAuthoritative
	}
}

func classifyConnectFailure(reason ConnectUnavailableReason) failureClass {
	switch reason {
	case TunnelUnavailable, AcceleratorUnavailable:
		return failureTemporary
	case ConnectVersionMismatch:
		return failureVersionMismatch
	case ConnectCancelled, WorkloadDisposing:
		return failureCancelled
	default:
		return failureAuthoritative
	}
}

func classifyResumeFailure(reason ResumeReason) failureClass {
	switch reason {
	case ResumeTransportUnavailable:
		return failureTemporary
	case ResumeVersionMismatch:
		return failureVersionMismatch
	case ResumeCancelled, ResumeWorkloadDisposing:
		return failureCancelled
	default:
		return failureAuthoritative
	}
}
