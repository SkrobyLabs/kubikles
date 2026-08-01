package k8s

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"kubikles/pkg/agent"

	authv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const acceleratorCapabilityReviewTimeout = 2 * time.Second
const maxCapabilityDiagnosticReasonRunes = 256

type selfSubjectAccessReviewCreator interface {
	Create(context.Context, *authv1.SelfSubjectAccessReview, metav1.CreateOptions) (*authv1.SelfSubjectAccessReview, error)
}

type capabilityRequirement struct {
	Capability agent.Capability
	Actions    []agent.ResourceActionID
}

var acceleratorCapabilityRequirements = []capabilityRequirement{
	{Capability: agent.CapabilitySecretsList, Actions: []agent.ResourceActionID{agent.ResourceActionCoreV1SecretsList}},
	{Capability: agent.CapabilitySecretsDetail, Actions: []agent.ResourceActionID{agent.ResourceActionCoreV1SecretsGet}},
	{Capability: agent.CapabilitySecretsWatch, Actions: []agent.ResourceActionID{agent.ResourceActionCoreV1SecretsWatch}},
}

type secretCapabilityResolver struct {
	creator selfSubjectAccessReviewCreator
	timeout time.Duration
}

var _ agent.CapabilityResolver = (*secretCapabilityResolver)(nil)
var _ agent.CapabilityResolverFactory = NewSecretCapabilityResolverFactory(nil)

// NewSecretCapabilityResolver creates a resolver using the Client's current typed client identity.
func NewSecretCapabilityResolver(client *Client) agent.CapabilityResolver {
	if client == nil {
		return newSecretCapabilityResolver(nil, acceleratorCapabilityReviewTimeout)
	}
	clientset, err := client.getClientset()
	if err != nil || clientset == nil {
		return newSecretCapabilityResolver(nil, acceleratorCapabilityReviewTimeout)
	}
	return newSecretCapabilityResolver(clientset.AuthorizationV1().SelfSubjectAccessReviews(), acceleratorCapabilityReviewTimeout)
}

// NewSecretCapabilityResolverFactory creates the narrow factory used by downstream composition.
func NewSecretCapabilityResolverFactory(client *Client) agent.CapabilityResolverFactory {
	return func() agent.CapabilityResolver { return NewSecretCapabilityResolver(client) }
}

func newSecretCapabilityResolver(creator selfSubjectAccessReviewCreator, timeout time.Duration) *secretCapabilityResolver {
	return &secretCapabilityResolver{creator: creator, timeout: timeout}
}

// ResolveCapabilities reviews every fixed Secret action in declared order and fails closed.
func (r *secretCapabilityResolver) ResolveCapabilities(ctx context.Context) agent.CapabilityResolution {
	resolution := agent.CapabilityResolution{
		Capabilities: make([]agent.Capability, 0, len(acceleratorCapabilityRequirements)),
		Diagnostics:  make([]agent.CapabilityDiagnostic, 0, len(acceleratorCapabilityRequirements)),
	}
	for _, requirement := range acceleratorCapabilityRequirements {
		if r == nil || r.creator == nil {
			for _, action := range requirement.Actions {
				resolution.Diagnostics = append(resolution.Diagnostics, unavailableDiagnostic(requirement.Capability, action))
			}
			continue
		}
		if r.resolveRequirement(ctx, requirement, &resolution) {
			resolution.Capabilities = append(resolution.Capabilities, requirement.Capability)
		}
	}
	return resolution
}

func (r *secretCapabilityResolver) resolveRequirement(ctx context.Context, requirement capabilityRequirement, resolution *agent.CapabilityResolution) bool {
	allowed := true
	for _, action := range requirement.Actions {
		diagnostic := r.resolveAction(ctx, requirement.Capability, action)
		resolution.Diagnostics = append(resolution.Diagnostics, diagnostic)
		if diagnostic.Outcome != agent.CapabilityCheckOutcomeAllowed {
			allowed = false
		}
	}
	return allowed
}

func unavailableDiagnostic(capability agent.Capability, action agent.ResourceActionID) agent.CapabilityDiagnostic {
	return agent.CapabilityDiagnostic{
		Capability: capability, Action: action, Outcome: agent.CapabilityCheckOutcomeAPIError,
		ErrorCode: agent.CapabilityCheckErrorCodeClientUnavailable,
	}
}

func (r *secretCapabilityResolver) resolveAction(callerContext context.Context, capability agent.Capability, action agent.ResourceActionID) agent.CapabilityDiagnostic {
	attributes, ok := resourceAttributesForAcceleratorAction(action)
	if !ok {
		return agent.CapabilityDiagnostic{Capability: capability, Action: action, Outcome: agent.CapabilityCheckOutcomeMalformed, ErrorCode: agent.CapabilityCheckErrorCodeMalformedResponse}
	}
	expectedAttributes := *attributes
	timeout := r.timeout
	if timeout <= 0 {
		timeout = acceleratorCapabilityReviewTimeout
	}
	ctx, cancel := context.WithTimeout(callerContext, timeout)
	defer cancel()
	review, err := r.creator.Create(ctx, &authv1.SelfSubjectAccessReview{
		TypeMeta: metav1.TypeMeta{APIVersion: "authorization.k8s.io/v1", Kind: "SelfSubjectAccessReview"},
		Spec:     authv1.SelfSubjectAccessReviewSpec{ResourceAttributes: attributes},
	}, metav1.CreateOptions{})
	if err != nil {
		return createErrorDiagnostic(capability, action, err)
	}
	if err := ctx.Err(); err != nil {
		return createErrorDiagnostic(capability, action, err)
	}
	return reviewDiagnostic(capability, action, &expectedAttributes, review)
}

func resourceAttributesForAcceleratorAction(action agent.ResourceActionID) (*authv1.ResourceAttributes, bool) {
	verb := ""
	switch action {
	case agent.ResourceActionCoreV1SecretsList:
		verb = "list"
	case agent.ResourceActionCoreV1SecretsGet:
		verb = "get"
	case agent.ResourceActionCoreV1SecretsWatch:
		verb = "watch"
	default:
		return nil, false
	}
	return &authv1.ResourceAttributes{Group: "", Version: "v1", Resource: "secrets", Namespace: "", Name: "", Verb: verb}, true
}

func createErrorDiagnostic(capability agent.Capability, action agent.ResourceActionID, err error) agent.CapabilityDiagnostic {
	diagnostic := agent.CapabilityDiagnostic{Capability: capability, Action: action, Outcome: agent.CapabilityCheckOutcomeAPIError, ErrorCode: agent.CapabilityCheckErrorCodeAPIError}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		diagnostic.Outcome = agent.CapabilityCheckOutcomeTimeout
		diagnostic.ErrorCode = agent.CapabilityCheckErrorCodeTimeout
	case errors.Is(err, context.Canceled):
		diagnostic.ErrorCode = agent.CapabilityCheckErrorCodeCanceled
	case apierrors.ReasonForError(err) == metav1.StatusReasonForbidden:
		diagnostic.ErrorCode = agent.CapabilityCheckErrorCodeForbidden
	case apierrors.ReasonForError(err) == metav1.StatusReasonUnauthorized:
		diagnostic.ErrorCode = agent.CapabilityCheckErrorCodeUnauthorized
	}
	return diagnostic
}

func reviewDiagnostic(capability agent.Capability, action agent.ResourceActionID, expected *authv1.ResourceAttributes, review *authv1.SelfSubjectAccessReview) agent.CapabilityDiagnostic {
	diagnostic := agent.CapabilityDiagnostic{Capability: capability, Action: action}
	if review == nil || review.Spec.ResourceAttributes == nil || review.Spec.NonResourceAttributes != nil || !resourceAttributesEqual(expected, review.Spec.ResourceAttributes) {
		diagnostic.Outcome = agent.CapabilityCheckOutcomeMalformed
		diagnostic.ErrorCode = agent.CapabilityCheckErrorCodeMalformedResponse
		return diagnostic
	}
	status := review.Status
	diagnostic.Reason = boundedDiagnosticReason(status.Reason)
	if status.EvaluationError != "" {
		if diagnostic.Reason == "" {
			diagnostic.Reason = boundedDiagnosticReason(status.EvaluationError)
		}
		diagnostic.Outcome = agent.CapabilityCheckOutcomeIncomplete
		return diagnostic
	}
	if status.Allowed && status.Denied {
		diagnostic.Outcome = agent.CapabilityCheckOutcomeMalformed
		diagnostic.ErrorCode = agent.CapabilityCheckErrorCodeMalformedResponse
		return diagnostic
	}
	if status.Allowed {
		diagnostic.Outcome = agent.CapabilityCheckOutcomeAllowed
		return diagnostic
	}
	if status.Denied || diagnostic.Reason != "" {
		diagnostic.Outcome = agent.CapabilityCheckOutcomeDenied
		return diagnostic
	}
	diagnostic.Outcome = agent.CapabilityCheckOutcomeIncomplete
	return diagnostic
}

func resourceAttributesEqual(expected, actual *authv1.ResourceAttributes) bool {
	return actual != nil &&
		expected.Group == actual.Group && expected.Version == actual.Version && expected.Resource == actual.Resource &&
		expected.Namespace == actual.Namespace && expected.Name == actual.Name && expected.Verb == actual.Verb &&
		expected.Subresource == actual.Subresource && actual.FieldSelector == nil && actual.LabelSelector == nil
}

func boundedDiagnosticReason(value string) string {
	value = strings.ToValidUTF8(value, "")
	if utf8.RuneCountInString(value) <= maxCapabilityDiagnosticReasonRunes {
		return value
	}
	var builder strings.Builder
	builder.Grow(maxCapabilityDiagnosticReasonRunes * utf8.UTFMax)
	count := 0
	for _, runeValue := range value {
		if count == maxCapabilityDiagnosticReasonRunes {
			break
		}
		builder.WriteRune(runeValue)
		count++
	}
	return builder.String()
}
