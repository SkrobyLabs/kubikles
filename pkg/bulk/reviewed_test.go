package bulk

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
)

// reviewedBulkMethods is the set of App methods the allowlist admits today.
// Each was read and confirmed to be read-only. A new List* / Get*Yaml /
// Get*History method that the naming rules admit makes
// TestAllowedMethodsMatchReviewedList fail until someone reviews it and adds
// it here (or adds it to denied in allowlist.go).
//
// Review notes: GetSecretYaml returns secret data by design (token protected);
// the Prometheus-backed history calls only query through the API server proxy.
var reviewedBulkMethods = []string{
	"GetAllContainersLogsAfter",
	"GetAllContainersLogsBefore",
	"GetAllPodsLogsAfter",
	"GetAllPodsLogsBefore",
	"GetCRDYaml",
	"GetCSIDriverYaml",
	"GetCSINodeYaml",
	"GetClusterRoleBindingYaml",
	"GetClusterRoleYaml",
	"GetConfigMapYaml",
	"GetControllerMetricsHistory",
	"GetControllerMetricsHistoryRange",
	"GetCronJobYaml",
	"GetCustomResourceYaml",
	"GetDaemonSetYaml",
	"GetDeploymentYaml",
	"GetEndpointSliceYaml",
	"GetEndpointsYaml",
	"GetEventYAML",
	"GetHPAYaml",
	"GetHelmReleaseHistory",
	"GetIngressClassYaml",
	"GetIngressYaml",
	"GetJobYaml",
	"GetLeaseYaml",
	"GetLimitRangeYaml",
	"GetMutatingWebhookConfigurationYaml",
	"GetNamespaceMetricsHistory",
	"GetNamespaceMetricsHistoryRange",
	"GetNamespaceYAML",
	"GetNetworkPolicyYaml",
	"GetNodeMemoryDiagnosticsHistory",
	"GetNodeMemoryDiagnosticsHistoryRange",
	"GetNodeMetrics",
	"GetNodeMetricsHistory",
	"GetNodeMetricsHistoryRange",
	"GetNodeYaml",
	"GetPDBYaml",
	"GetPVCYaml",
	"GetPVYaml",
	"GetPodLogsAfter",
	"GetPodLogsBefore",
	"GetPodMetrics",
	"GetPodMetricsHistory",
	"GetPodMetricsHistoryRange",
	"GetPodYaml",
	"GetPriorityClassYaml",
	"GetReplicaSetYaml",
	"GetResourceQuotaYaml",
	"GetRoleBindingYaml",
	"GetRoleYaml",
	"GetSecretYaml",
	"GetServiceAccountYaml",
	"GetServiceYaml",
	"GetStatefulSetYaml",
	"GetStorageClassYaml",
	"GetValidatingWebhookConfigurationYaml",
	"ListCRDs",
	"ListCSIDrivers",
	"ListCSINodes",
	"ListChartSources",
	"ListClusterRoleBindings",
	"ListClusterRoles",
	"ListConfigMaps",
	"ListContexts",
	"ListCronJobs",
	"ListCustomResources",
	"ListDaemonSets",
	"ListDeployments",
	"ListDeploymentsForContext",
	"ListEndpointSlices",
	"ListEndpoints",
	"ListEvents",
	"ListHPAs",
	"ListHelmReleases",
	"ListHelmRepositories",
	"ListIngressClasses",
	"ListIngresses",
	"ListIssueRules",
	"ListJobs",
	"ListLeases",
	"ListLimitRanges",
	"ListMutatingWebhookConfigurations",
	"ListNamespaces",
	"ListNamespacesForContext",
	"ListNetworkPolicies",
	"ListNodes",
	"ListOCIRegistries",
	"ListPDBs",
	"ListPVCs",
	"ListPVs",
	"ListPods",
	"ListPodsForContext",
	"ListPodsForNode",
	"ListPriorityClasses",
	"ListPrometheusInstalls",
	"ListReplicaSets",
	"ListResourceNamesForContext",
	"ListResourceQuotas",
	"ListRoleBindings",
	"ListRoles",
	"ListSecrets",
	"ListSecretsMetadata",
	"ListServiceAccounts",
	"ListServices",
	"ListStatefulSets",
	"ListStorageClasses",
	"ListValidatingWebhookConfigurations",
}

func TestAllowedMethodsMatchReviewedList(t *testing.T) {
	var got []string
	for _, n := range dispatchNames(t) {
		if IsAllowed(n) {
			got = append(got, n)
		}
	}
	want := append([]string(nil), reviewedBulkMethods...)
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("allowlisted dispatcher methods differ from the reviewed list.\n got: %v\nwant: %v", got, want)
	}
}

func TestListPodFilesIsDenied(t *testing.T) {
	if IsAllowed("ListPodFiles") {
		t.Fatal("ListPodFiles execs in the pod and must not be bulk routable")
	}
}

type panicCaller struct{}

func (panicCaller) CallMethod(string, []json.RawMessage) (interface{}, error) {
	panic("boom")
}

func TestPanicInCallerYields500(t *testing.T) {
	h := newTestHandler(panicCaller{}, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authed("GET", callURL("ListPods")))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code %d, want 500", rec.Code)
	}
}
