package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"kubikles/pkg/agent"

	authv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type capturedReviewCreator struct {
	mu        sync.Mutex
	requests  []*authv1.SelfSubjectAccessReview
	responses []*authv1.SelfSubjectAccessReview
	err       error
	create    func(context.Context, *authv1.SelfSubjectAccessReview) (*authv1.SelfSubjectAccessReview, error)
}

func (c *capturedReviewCreator) Create(ctx context.Context, review *authv1.SelfSubjectAccessReview, _ metav1.CreateOptions) (*authv1.SelfSubjectAccessReview, error) {
	c.mu.Lock()
	c.requests = append(c.requests, review.DeepCopy())
	index := len(c.requests) - 1
	c.mu.Unlock()
	if c.create != nil {
		return c.create(ctx, review)
	}
	if c.err != nil {
		return nil, c.err
	}
	return c.responses[index], nil
}

func allowResponse(request *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
	return &authv1.SelfSubjectAccessReview{Spec: request.Spec, Status: authv1.SubjectAccessReviewStatus{Allowed: true}}
}

func TestSecretCapabilityResolverExactRequests(t *testing.T) {
	creator := &capturedReviewCreator{create: func(_ context.Context, request *authv1.SelfSubjectAccessReview) (*authv1.SelfSubjectAccessReview, error) {
		return allowResponse(request), nil
	}}
	resolution := newSecretCapabilityResolver(creator, time.Second).ResolveCapabilities(context.Background())
	if got, want := resolution.Capabilities, []agent.Capability{agent.CapabilitySecretsList, agent.CapabilitySecretsDetail, agent.CapabilitySecretsWatch}; !equalCapabilities(got, want) {
		t.Fatalf("capabilities = %v, want %v", got, want)
	}
	if len(creator.requests) != 3 {
		t.Fatalf("create calls = %d, want 3", len(creator.requests))
	}
	for index, verb := range []string{"list", "get", "watch"} {
		request := creator.requests[index]
		if request.APIVersion != "authorization.k8s.io/v1" || request.Kind != "SelfSubjectAccessReview" {
			t.Errorf("request %d type = %s %s", index, request.APIVersion, request.Kind)
		}
		attributes := request.Spec.ResourceAttributes
		if attributes == nil || attributes.Group != "" || attributes.Version != "v1" || attributes.Resource != "secrets" || attributes.Verb != verb || attributes.Namespace != "" || attributes.Name != "" {
			t.Errorf("request %d attributes = %#v", index, attributes)
		}
		if request.Spec.NonResourceAttributes != nil {
			t.Errorf("request %d has non-resource attributes", index)
		}
	}
}

func TestSecretCapabilityResolverAdvertisesOnlyCompleteAllows(t *testing.T) {
	for _, test := range []struct {
		name    string
		allowed []bool
		want    []agent.Capability
	}{
		{"all", []bool{true, true, true}, []agent.Capability{agent.CapabilitySecretsList, agent.CapabilitySecretsDetail, agent.CapabilitySecretsWatch}},
		{"list only", []bool{true, false, false}, []agent.Capability{agent.CapabilitySecretsList}},
		{"detail only", []bool{false, true, false}, []agent.Capability{agent.CapabilitySecretsDetail}},
		{"watch only", []bool{false, false, true}, []agent.Capability{agent.CapabilitySecretsWatch}},
		{"list detail", []bool{true, true, false}, []agent.Capability{agent.CapabilitySecretsList, agent.CapabilitySecretsDetail}},
		{"none", []bool{false, false, false}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			index := 0
			creator := &capturedReviewCreator{create: func(_ context.Context, request *authv1.SelfSubjectAccessReview) (*authv1.SelfSubjectAccessReview, error) {
				allowed := test.allowed[index]
				index++
				return &authv1.SelfSubjectAccessReview{Spec: request.Spec, Status: authv1.SubjectAccessReviewStatus{Allowed: allowed, Denied: !allowed}}, nil
			}}
			resolution := newSecretCapabilityResolver(creator, time.Second).ResolveCapabilities(context.Background())
			if !equalCapabilities(resolution.Capabilities, test.want) || len(resolution.Diagnostics) != 3 {
				t.Errorf("resolution = %#v, want capabilities %v and three diagnostics", resolution, test.want)
			}
		})
	}

	resolution := &agent.CapabilityResolution{}
	resolver := newSecretCapabilityResolver(&capturedReviewCreator{create: func(_ context.Context, request *authv1.SelfSubjectAccessReview) (*authv1.SelfSubjectAccessReview, error) {
		return &authv1.SelfSubjectAccessReview{Spec: request.Spec, Status: authv1.SubjectAccessReviewStatus{Denied: true}}, nil
	}}, time.Second)
	if resolver.resolveRequirement(context.Background(), capabilityRequirement{Capability: agent.CapabilitySecretsList, Actions: []agent.ResourceActionID{agent.ResourceActionCoreV1SecretsList, agent.ResourceActionCoreV1SecretsGet}}, resolution) {
		t.Error("multi-action requirement was advertised with denied members")
	}
}

func TestSecretCapabilityResolverFailsClosedOnReviewStatus(t *testing.T) {
	for _, test := range []struct {
		name     string
		response func(*authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview
		outcome  agent.CapabilityCheckOutcome
		code     agent.CapabilityCheckErrorCode
	}{
		{"denied", func(r *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
			return &authv1.SelfSubjectAccessReview{Spec: r.Spec, Status: authv1.SubjectAccessReviewStatus{Denied: true}}
		}, agent.CapabilityCheckOutcomeDenied, ""},
		{"incomplete", func(r *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
			return &authv1.SelfSubjectAccessReview{Spec: r.Spec}
		}, agent.CapabilityCheckOutcomeIncomplete, ""},
		{"evaluation", func(r *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
			return &authv1.SelfSubjectAccessReview{Spec: r.Spec, Status: authv1.SubjectAccessReviewStatus{EvaluationError: "incomplete"}}
		}, agent.CapabilityCheckOutcomeIncomplete, ""},
		{"allowed with evaluation error", func(r *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
			return &authv1.SelfSubjectAccessReview{Spec: r.Spec, Status: authv1.SubjectAccessReviewStatus{Allowed: true, EvaluationError: "incomplete"}}
		}, agent.CapabilityCheckOutcomeIncomplete, ""},
		{"denied by reason", func(r *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
			return &authv1.SelfSubjectAccessReview{Spec: r.Spec, Status: authv1.SubjectAccessReviewStatus{Reason: "no matching binding"}}
		}, agent.CapabilityCheckOutcomeDenied, ""},
		{"contradiction", func(r *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
			return &authv1.SelfSubjectAccessReview{Spec: r.Spec, Status: authv1.SubjectAccessReviewStatus{Allowed: true, Denied: true}}
		}, agent.CapabilityCheckOutcomeMalformed, agent.CapabilityCheckErrorCodeMalformedResponse},
		{"nil response", func(*authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview { return nil }, agent.CapabilityCheckOutcomeMalformed, agent.CapabilityCheckErrorCodeMalformedResponse},
		{"nil resource attributes", func(r *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
			return &authv1.SelfSubjectAccessReview{Spec: authv1.SelfSubjectAccessReviewSpec{}, Status: authv1.SubjectAccessReviewStatus{Allowed: true}}
		}, agent.CapabilityCheckOutcomeMalformed, agent.CapabilityCheckErrorCodeMalformedResponse},
		{"resource and non-resource attributes", func(r *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
			result := allowResponse(r)
			result.Spec.NonResourceAttributes = &authv1.NonResourceAttributes{Verb: "get", Path: "/healthz"}
			return result
		}, agent.CapabilityCheckOutcomeMalformed, agent.CapabilityCheckErrorCodeMalformedResponse},
		{"mismatched group", func(r *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
			result := allowResponse(r)
			result.Spec.ResourceAttributes.Group = "apps"
			return result
		}, agent.CapabilityCheckOutcomeMalformed, agent.CapabilityCheckErrorCodeMalformedResponse},
		{"mismatched version", func(r *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
			result := allowResponse(r)
			result.Spec.ResourceAttributes.Version = "v2"
			return result
		}, agent.CapabilityCheckOutcomeMalformed, agent.CapabilityCheckErrorCodeMalformedResponse},
		{"mismatched resource", func(r *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
			result := allowResponse(r)
			result.Spec.ResourceAttributes.Resource = "configmaps"
			return result
		}, agent.CapabilityCheckOutcomeMalformed, agent.CapabilityCheckErrorCodeMalformedResponse},
		{"mismatched verb", func(r *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
			result := allowResponse(r)
			result.Spec.ResourceAttributes.Verb = "delete"
			return result
		}, agent.CapabilityCheckOutcomeMalformed, agent.CapabilityCheckErrorCodeMalformedResponse},
		{"mismatched namespace", func(r *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
			result := allowResponse(r)
			result.Spec.ResourceAttributes.Namespace = "default"
			return result
		}, agent.CapabilityCheckOutcomeMalformed, agent.CapabilityCheckErrorCodeMalformedResponse},
		{"mismatched name", func(r *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
			result := allowResponse(r)
			result.Spec.ResourceAttributes.Name = "example"
			return result
		}, agent.CapabilityCheckOutcomeMalformed, agent.CapabilityCheckErrorCodeMalformedResponse},
		{"mismatched subresource", func(r *authv1.SelfSubjectAccessReview) *authv1.SelfSubjectAccessReview {
			result := allowResponse(r)
			result.Spec.ResourceAttributes.Subresource = "status"
			return result
		}, agent.CapabilityCheckOutcomeMalformed, agent.CapabilityCheckErrorCodeMalformedResponse},
	} {
		t.Run(test.name, func(t *testing.T) {
			creator := &capturedReviewCreator{create: func(_ context.Context, r *authv1.SelfSubjectAccessReview) (*authv1.SelfSubjectAccessReview, error) {
				return test.response(r), nil
			}}
			resolution := newSecretCapabilityResolver(creator, time.Second).ResolveCapabilities(context.Background())
			assertFailClosedDiagnostics(t, resolution, test.outcome, test.code)
		})
	}
}

func TestSecretCapabilityResolverClassifiesCreateErrorsSafely(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		code agent.CapabilityCheckErrorCode
	}{
		{"forbidden", apierrors.NewForbidden(schema.GroupResource{Group: "authorization.k8s.io", Resource: "selfsubjectaccessreviews"}, "", errors.New("Bearer super-secret-token")), agent.CapabilityCheckErrorCodeForbidden},
		{"unauthorized", apierrors.NewUnauthorized("Bearer super-secret-token"), agent.CapabilityCheckErrorCodeUnauthorized},
		{"generic", errors.New("Bearer super-secret-token"), agent.CapabilityCheckErrorCodeAPIError},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolution := newSecretCapabilityResolver(&capturedReviewCreator{err: test.err}, time.Second).ResolveCapabilities(context.Background())
			if resolution.Diagnostics[0].ErrorCode != test.code || strings.Contains(string(mustJSON(t, resolution)), "Bearer") {
				t.Errorf("unsafe/error classification: %#v", resolution)
			}
		})
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	resolution := newSecretCapabilityResolver(&capturedReviewCreator{create: func(ctx context.Context, _ *authv1.SelfSubjectAccessReview) (*authv1.SelfSubjectAccessReview, error) {
		return nil, ctx.Err()
	}}, time.Second).ResolveCapabilities(canceled)
	if resolution.Diagnostics[0].ErrorCode != agent.CapabilityCheckErrorCodeCanceled {
		t.Errorf("canceled = %#v", resolution.Diagnostics[0])
	}

	t.Run("pre-canceled successful create", func(t *testing.T) {
		callerContext, cancel := context.WithCancel(context.Background())
		cancel()
		creator := &capturedReviewCreator{create: func(_ context.Context, request *authv1.SelfSubjectAccessReview) (*authv1.SelfSubjectAccessReview, error) {
			return allowResponse(request), nil
		}}
		resolution := newSecretCapabilityResolver(creator, time.Second).ResolveCapabilities(callerContext)
		assertFailClosedDiagnostics(t, resolution, agent.CapabilityCheckOutcomeAPIError, agent.CapabilityCheckErrorCodeCanceled)
	})
}

func TestSecretCapabilityResolverTimesOutAndBoundsDiagnostics(t *testing.T) {
	blocker := &capturedReviewCreator{create: func(ctx context.Context, _ *authv1.SelfSubjectAccessReview) (*authv1.SelfSubjectAccessReview, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	t.Run("resolver timeout", func(t *testing.T) {
		start := time.Now()
		resolution := newSecretCapabilityResolver(blocker, 5*time.Millisecond).ResolveCapabilities(context.Background())
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("resolution took %v, want prompt completion", elapsed)
		}
		assertFailClosedDiagnostics(t, resolution, agent.CapabilityCheckOutcomeTimeout, agent.CapabilityCheckErrorCodeTimeout)
	})

	t.Run("resolver timeout with successful create", func(t *testing.T) {
		creator := &capturedReviewCreator{create: func(ctx context.Context, request *authv1.SelfSubjectAccessReview) (*authv1.SelfSubjectAccessReview, error) {
			<-ctx.Done()
			return allowResponse(request), nil
		}}
		resolution := newSecretCapabilityResolver(creator, 5*time.Millisecond).ResolveCapabilities(context.Background())
		assertFailClosedDiagnostics(t, resolution, agent.CapabilityCheckOutcomeTimeout, agent.CapabilityCheckErrorCodeTimeout)
	})

	t.Run("earlier caller deadline", func(t *testing.T) {
		callerContext, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()
		start := time.Now()
		resolution := newSecretCapabilityResolver(blocker, time.Second).ResolveCapabilities(callerContext)
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Fatalf("resolution took %v, want caller deadline to win promptly", elapsed)
		}
		assertFailClosedDiagnostics(t, resolution, agent.CapabilityCheckOutcomeTimeout, agent.CapabilityCheckErrorCodeTimeout)
	})

	t.Run("long reason", func(t *testing.T) {
		longReason := strings.Repeat("界", 300)
		creator := &capturedReviewCreator{create: func(_ context.Context, r *authv1.SelfSubjectAccessReview) (*authv1.SelfSubjectAccessReview, error) {
			return &authv1.SelfSubjectAccessReview{Spec: r.Spec, Status: authv1.SubjectAccessReviewStatus{Reason: longReason}}, nil
		}}
		resolution := newSecretCapabilityResolver(creator, time.Second).ResolveCapabilities(context.Background())
		assertFailClosedDiagnostics(t, resolution, agent.CapabilityCheckOutcomeDenied, agent.CapabilityCheckErrorCodeNone)
		for index, diagnostic := range resolution.Diagnostics {
			if !utf8.ValidString(diagnostic.Reason) || utf8.RuneCountInString(diagnostic.Reason) != 256 {
				t.Errorf("diagnostic %d reason = %q, want valid UTF-8 bounded to 256 runes", index, diagnostic.Reason)
			}
		}
	})

	t.Run("invalid UTF-8", func(t *testing.T) {
		invalidReason := string([]byte{'b', 'e', 'f', 'o', 'r', 'e', 0xff, 0xfe, 'a', 'f', 't', 'e', 'r'})
		creator := &capturedReviewCreator{create: func(_ context.Context, r *authv1.SelfSubjectAccessReview) (*authv1.SelfSubjectAccessReview, error) {
			return &authv1.SelfSubjectAccessReview{Spec: r.Spec, Status: authv1.SubjectAccessReviewStatus{Reason: invalidReason}}, nil
		}}
		resolution := newSecretCapabilityResolver(creator, time.Second).ResolveCapabilities(context.Background())
		assertFailClosedDiagnostics(t, resolution, agent.CapabilityCheckOutcomeDenied, agent.CapabilityCheckErrorCodeNone)
		for index, diagnostic := range resolution.Diagnostics {
			if !utf8.ValidString(diagnostic.Reason) || diagnostic.Reason != "beforeafter" {
				t.Errorf("diagnostic %d reason = %q, want sanitized valid UTF-8", index, diagnostic.Reason)
			}
		}
	})

	t.Run("reason preferred over evaluation error", func(t *testing.T) {
		creator := &capturedReviewCreator{create: func(_ context.Context, r *authv1.SelfSubjectAccessReview) (*authv1.SelfSubjectAccessReview, error) {
			return &authv1.SelfSubjectAccessReview{Spec: r.Spec, Status: authv1.SubjectAccessReviewStatus{Reason: "preferred reason", EvaluationError: "fallback evaluation error"}}, nil
		}}
		resolution := newSecretCapabilityResolver(creator, time.Second).ResolveCapabilities(context.Background())
		assertFailClosedDiagnostics(t, resolution, agent.CapabilityCheckOutcomeIncomplete, agent.CapabilityCheckErrorCodeNone)
		for index, diagnostic := range resolution.Diagnostics {
			if diagnostic.Reason != "preferred reason" {
				t.Errorf("diagnostic %d reason = %q, want deterministic status reason", index, diagnostic.Reason)
			}
		}
	})

	t.Run("evaluation error fallback", func(t *testing.T) {
		longEvaluationError := strings.Repeat("界", 300)
		creator := &capturedReviewCreator{create: func(_ context.Context, r *authv1.SelfSubjectAccessReview) (*authv1.SelfSubjectAccessReview, error) {
			return &authv1.SelfSubjectAccessReview{Spec: r.Spec, Status: authv1.SubjectAccessReviewStatus{EvaluationError: longEvaluationError}}, nil
		}}
		resolution := newSecretCapabilityResolver(creator, time.Second).ResolveCapabilities(context.Background())
		assertFailClosedDiagnostics(t, resolution, agent.CapabilityCheckOutcomeIncomplete, agent.CapabilityCheckErrorCodeNone)
		for index, diagnostic := range resolution.Diagnostics {
			if !utf8.ValidString(diagnostic.Reason) || utf8.RuneCountInString(diagnostic.Reason) != 256 {
				t.Errorf("diagnostic %d evaluation fallback = %q, want valid UTF-8 bounded to 256 runes", index, diagnostic.Reason)
			}
		}
	})
}

func TestBoundedDiagnosticReasonAllocationIsBounded(t *testing.T) {
	largeReason := strings.Repeat("x", 2*1024*1024)
	var bounded string
	result := testing.Benchmark(func(b *testing.B) {
		for range b.N {
			bounded = boundedDiagnosticReason(largeReason)
		}
	})

	if got := utf8.RuneCountInString(bounded); got != maxCapabilityDiagnosticReasonRunes {
		t.Fatalf("bounded reason runes = %d, want %d", got, maxCapabilityDiagnosticReasonRunes)
	}
	if allocated := result.AllocedBytesPerOp(); allocated > 8*1024 {
		t.Fatalf("allocated bytes per operation = %d, want at most 8192 for fixed-size output", allocated)
	}
}

func TestNewSecretCapabilityResolverFactoryAndUnavailableClient(t *testing.T) {
	var factory agent.CapabilityResolverFactory = NewSecretCapabilityResolverFactory(nil)
	if factory() == nil {
		t.Fatal("factory returned nil")
	}
	for _, client := range []*Client{nil, {}} {
		resolution := NewSecretCapabilityResolver(client).ResolveCapabilities(context.Background())
		if len(resolution.Capabilities) != 0 || len(resolution.Diagnostics) != 3 {
			t.Errorf("unavailable resolution = %#v", resolution)
			continue
		}
		for _, diagnostic := range resolution.Diagnostics {
			if diagnostic.ErrorCode != agent.CapabilityCheckErrorCodeClientUnavailable {
				t.Errorf("diagnostic = %#v", diagnostic)
			}
		}
	}
}

func equalCapabilities(got, want []agent.Capability) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func assertFailClosedDiagnostics(t *testing.T, resolution agent.CapabilityResolution, outcome agent.CapabilityCheckOutcome, code agent.CapabilityCheckErrorCode) {
	t.Helper()
	if len(resolution.Capabilities) != 0 {
		t.Errorf("capabilities = %v, want none", resolution.Capabilities)
	}
	wantCapabilities := []agent.Capability{agent.CapabilitySecretsList, agent.CapabilitySecretsDetail, agent.CapabilitySecretsWatch}
	wantActions := []agent.ResourceActionID{agent.ResourceActionCoreV1SecretsList, agent.ResourceActionCoreV1SecretsGet, agent.ResourceActionCoreV1SecretsWatch}
	if len(resolution.Diagnostics) != len(wantCapabilities) {
		t.Fatalf("diagnostics = %d, want %d", len(resolution.Diagnostics), len(wantCapabilities))
	}
	for index, diagnostic := range resolution.Diagnostics {
		if diagnostic.Capability != wantCapabilities[index] || diagnostic.Action != wantActions[index] || diagnostic.Outcome != outcome || diagnostic.ErrorCode != code {
			t.Errorf("diagnostic %d = %#v, want capability %q, action %q, outcome %q, code %q", index, diagnostic, wantCapabilities[index], wantActions[index], outcome, code)
		}
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
