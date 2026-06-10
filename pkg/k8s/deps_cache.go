// Code split from dependencies.go; see that file for the graph types and entry point.
package k8s

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// resourceCache provides request-scoped caching for Kubernetes resources
// to avoid duplicate API calls within a single GetResourceDependencies call
type resourceCache struct {
	pods        map[string][]v1.Pod               // namespace -> pods
	services    map[string][]v1.Service           // namespace -> services
	ingresses   map[string][]networkingv1.Ingress // namespace -> ingresses
	replicaSets map[string][]appsv1.ReplicaSet    // namespace -> replicasets
	jobs        map[string][]batchv1.Job          // namespace -> jobs
	// Name-indexed maps for O(1) lookups by name
	podsByName        map[string]map[string]*v1.Pod            // namespace -> name -> pod
	replicaSetsByName map[string]map[string]*appsv1.ReplicaSet // namespace -> name -> replicaset
	jobsByName        map[string]map[string]*batchv1.Job       // namespace -> name -> job
	// Cluster-wide caches for cluster-scoped resource queries
	allPods            []v1.Pod
	allPodsCached      bool
	allIngresses       []networkingv1.Ingress
	allIngressesCached bool
	ctx                context.Context // Request context with timeout
	cs                 kubernetes.Interface
	// Dynamic client for CRD owner resolution
	dc dynamic.Interface
	// GVR cache: "apiVersion/kind" -> resolved GVR (avoids repeated discovery calls)
	gvrCache map[string]*schema.GroupVersionResource
	// CRD resource cache: "kind/namespace/name" -> fetched unstructured resource
	crdCache map[string]*unstructured.Unstructured
}

func newResourceCache(ctx context.Context, cs kubernetes.Interface, dc dynamic.Interface) *resourceCache {
	return &resourceCache{
		pods:              make(map[string][]v1.Pod),
		services:          make(map[string][]v1.Service),
		ingresses:         make(map[string][]networkingv1.Ingress),
		replicaSets:       make(map[string][]appsv1.ReplicaSet),
		jobs:              make(map[string][]batchv1.Job),
		podsByName:        make(map[string]map[string]*v1.Pod),
		replicaSetsByName: make(map[string]map[string]*appsv1.ReplicaSet),
		jobsByName:        make(map[string]map[string]*batchv1.Job),
		ctx:               ctx,
		cs:                cs,
		dc:                dc,
		gvrCache:          make(map[string]*schema.GroupVersionResource),
		crdCache:          make(map[string]*unstructured.Unstructured),
	}
}

// resolveGVR resolves an apiVersion + kind to a GroupVersionResource using the discovery API.
// Results are cached per request to avoid repeated discovery calls.
func (rc *resourceCache) resolveGVR(apiVersion, kind string) (*schema.GroupVersionResource, error) {
	cacheKey := apiVersion + "/" + kind
	if cached, ok := rc.gvrCache[cacheKey]; ok {
		return cached, nil
	}

	// Parse apiVersion into group and version
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid apiVersion %q: %w", apiVersion, err)
	}

	// Use discovery to find the resource name for this kind
	resourceList, err := rc.cs.Discovery().ServerResourcesForGroupVersion(apiVersion)
	if err != nil {
		return nil, fmt.Errorf("discovery failed for %s: %w", apiVersion, err)
	}

	// Find the resource matching the kind (use the plural resource name)
	lowerKind := strings.ToLower(kind)
	for _, r := range resourceList.APIResources {
		// Skip subresources (e.g., pods/status)
		if strings.Contains(r.Name, "/") {
			continue
		}
		if r.Kind == kind {
			result := &schema.GroupVersionResource{
				Group:    gv.Group,
				Version:  gv.Version,
				Resource: r.Name,
			}
			rc.gvrCache[cacheKey] = result
			return result, nil
		}
		// Fallback: match by lowercase plural name (e.g., "rollouts" for "Rollout")
		if r.Name == lowerKind+"s" || r.Name == lowerKind+"es" || r.Name == lowerKind {
			result := &schema.GroupVersionResource{
				Group:    gv.Group,
				Version:  gv.Version,
				Resource: r.Name,
			}
			rc.gvrCache[cacheKey] = result
			return result, nil
		}
	}

	return nil, fmt.Errorf("resource %q not found in %s", kind, apiVersion)
}

// getCRDResource fetches a CRD resource using the dynamic client.
// Tries namespaced first, then cluster-scoped. Results are cached per request.
func (rc *resourceCache) getCRDResource(gvr *schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	cacheKey := gvr.Resource + "/" + namespace + "/" + name
	if cached, ok := rc.crdCache[cacheKey]; ok {
		return cached, nil
	}

	if rc.dc == nil {
		return nil, fmt.Errorf("dynamic client not available")
	}

	var obj *unstructured.Unstructured
	var err error

	// Try namespaced first if namespace is provided
	if namespace != "" {
		obj, err = rc.dc.Resource(*gvr).Namespace(namespace).Get(rc.ctx, name, metav1.GetOptions{})
	}

	// If namespaced failed or no namespace, try cluster-scoped
	if err != nil || namespace == "" {
		obj, err = rc.dc.Resource(*gvr).Get(rc.ctx, name, metav1.GetOptions{})
	}

	if err != nil {
		return nil, err
	}

	rc.crdCache[cacheKey] = obj
	return obj, nil
}

func (rc *resourceCache) getPods(namespace string) ([]v1.Pod, error) {
	if cached, ok := rc.pods[namespace]; ok {
		return cached, nil
	}
	list, err := rc.cs.CoreV1().Pods(namespace).List(rc.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rc.pods[namespace] = list.Items
	// Build name index for O(1) lookups
	rc.podsByName[namespace] = make(map[string]*v1.Pod, len(list.Items))
	for i := range list.Items {
		rc.podsByName[namespace][list.Items[i].Name] = &list.Items[i]
	}
	return list.Items, nil
}

// getPodByName returns a pod by name using the cached pod list, or fetches the list if not cached.
// Returns nil if pod not found (without error), error only if API call fails.
func (rc *resourceCache) getPodByName(namespace, name string) (*v1.Pod, error) {
	// Ensure cache is populated
	if _, ok := rc.pods[namespace]; !ok {
		if _, err := rc.getPods(namespace); err != nil {
			return nil, err
		}
	}
	// Lookup by name (O(1))
	if nameIndex, ok := rc.podsByName[namespace]; ok {
		return nameIndex[name], nil
	}
	return nil, nil //nolint:nilnil // nil means "not found", not an error
}

func (rc *resourceCache) getServices(namespace string) ([]v1.Service, error) {
	if cached, ok := rc.services[namespace]; ok {
		return cached, nil
	}
	list, err := rc.cs.CoreV1().Services(namespace).List(rc.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rc.services[namespace] = list.Items
	return list.Items, nil
}

func (rc *resourceCache) getIngresses(namespace string) ([]networkingv1.Ingress, error) {
	if cached, ok := rc.ingresses[namespace]; ok {
		return cached, nil
	}
	list, err := rc.cs.NetworkingV1().Ingresses(namespace).List(rc.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rc.ingresses[namespace] = list.Items
	return list.Items, nil
}

func (rc *resourceCache) getReplicaSets(namespace string) ([]appsv1.ReplicaSet, error) {
	if cached, ok := rc.replicaSets[namespace]; ok {
		return cached, nil
	}
	list, err := rc.cs.AppsV1().ReplicaSets(namespace).List(rc.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rc.replicaSets[namespace] = list.Items
	// Build name-indexed map for O(1) lookups
	nameMap := make(map[string]*appsv1.ReplicaSet, len(list.Items))
	for i := range list.Items {
		nameMap[list.Items[i].Name] = &list.Items[i]
	}
	rc.replicaSetsByName[namespace] = nameMap
	return list.Items, nil
}

// getReplicaSetByName returns a ReplicaSet by name with O(1) lookup
func (rc *resourceCache) getReplicaSetByName(namespace, name string) (*appsv1.ReplicaSet, error) {
	// Ensure the cache is populated
	if _, ok := rc.replicaSetsByName[namespace]; !ok {
		if _, err := rc.getReplicaSets(namespace); err != nil {
			return nil, err
		}
	}
	if rs, ok := rc.replicaSetsByName[namespace][name]; ok {
		return rs, nil
	}
	return nil, nil //nolint:nilnil // nil means "not found", not an error
}

func (rc *resourceCache) getJobs(namespace string) ([]batchv1.Job, error) {
	if cached, ok := rc.jobs[namespace]; ok {
		return cached, nil
	}
	list, err := rc.cs.BatchV1().Jobs(namespace).List(rc.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rc.jobs[namespace] = list.Items
	// Build name-indexed map for O(1) lookups
	nameMap := make(map[string]*batchv1.Job, len(list.Items))
	for i := range list.Items {
		nameMap[list.Items[i].Name] = &list.Items[i]
	}
	rc.jobsByName[namespace] = nameMap
	return list.Items, nil
}

// getJobByName returns a Job by name with O(1) lookup
func (rc *resourceCache) getJobByName(namespace, name string) (*batchv1.Job, error) {
	// Ensure the cache is populated
	if _, ok := rc.jobsByName[namespace]; !ok {
		if _, err := rc.getJobs(namespace); err != nil {
			return nil, err
		}
	}
	if job, ok := rc.jobsByName[namespace][name]; ok {
		return job, nil
	}
	return nil, nil //nolint:nilnil // nil means "not found", not an error
}

// getAllPods returns all pods cluster-wide with caching
// Used for cluster-scoped resources like PriorityClass
func (rc *resourceCache) getAllPods() ([]v1.Pod, error) {
	if rc.allPodsCached {
		return rc.allPods, nil
	}
	list, err := rc.cs.CoreV1().Pods("").List(rc.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rc.allPods = list.Items
	rc.allPodsCached = true
	return list.Items, nil
}

// getAllIngresses returns all ingresses cluster-wide with caching
// Used for cluster-scoped resources like IngressClass
func (rc *resourceCache) getAllIngresses() ([]networkingv1.Ingress, error) {
	if rc.allIngressesCached {
		return rc.allIngresses, nil
	}
	list, err := rc.cs.NetworkingV1().Ingresses("").List(rc.ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	rc.allIngresses = list.Items
	rc.allIngressesCached = true
	return list.Items, nil
}
