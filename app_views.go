package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"

	"kubikles/pkg/debug"
	"kubikles/pkg/k8s"
	"kubikles/pkg/rows"
	"kubikles/pkg/store"
)

// envDisableViews switches the view protocol off (the kill switch); the frontend then keeps every
// list on the legacy hooks.
const envDisableViews = "KUBIKLES_VIEWS"

// objectFetchTimeout bounds a live GET behind GetResourceObject.
const objectFetchTimeout = 30 * time.Second

// initViews creates the view manager on top of the store and wires its notices to the
// event path. It runs right after initStore.
func (a *App) initViews() {
	if a.views != nil {
		a.views.Close()
	}
	if a.viewNotifier != nil {
		a.viewNotifier.Stop()
	}
	a.viewNotifier = newViewNotifier(
		func() noticeSource {
			if m := a.views; m != nil {
				return m
			}
			return nil
		},
		func() time.Duration {
			if a.eventCoalescer != nil {
				return a.eventCoalescer.FrameDelay()
			}
			return defaultNoticeDelay
		},
		a.emitEvent,
	)
	if a.overlays == nil {
		a.overlays = newOverlaySources()
		a.wireOverlayBackends(a.overlays)
	}
	a.views = rows.NewManager(rows.Config{
		Provider: rows.StoreProvider{M: a.store},
		Overlays: a.overlays.hub,
		Context:  a.GetCurrentContext,
		OnDirty:  a.viewNotifier.Mark,
		Broad:    a.broadObservation,
		CRD:      crdResolver{a},
	})
}

// crdResolver resolves custom resource scope and printer columns for OpenView.
type crdResolver struct{ a *App }

func (r crdResolver) ResolveCRD(contextName string, d rows.CRDDescriptor) (rows.CRDInfo, error) {
	if r.a.k8sClient == nil {
		return rows.CRDInfo{}, errNoK8sClient
	}
	info, err := r.a.k8sClient.ResolveCRDView(contextName, d.Group, d.Version, d.Resource)
	if err != nil {
		return rows.CRDInfo{}, err
	}
	out := rows.CRDInfo{Namespaced: info.Namespaced}
	for _, c := range info.Columns {
		out.Columns = append(out.Columns, rows.PrinterColumn{Name: c.Name, Type: c.Type, JSONPath: c.JSONPath, Priority: c.Priority})
	}
	return out, nil
}

// hookViewForeground flushes pending view notices the moment the window comes back, like the
// event coalescer flushes its own pending events.
func (a *App) hookViewForeground() {
	if a.eventCoalescer != nil {
		a.eventCoalescer.SetForegroundHook(func() {
			if a.viewNotifier != nil {
				a.viewNotifier.FlushNow()
			}
		})
	}
}

// broadObservation reports a warm cluster-wide shard of a type to the namespace planner.
func (a *App) broadObservation(contextName, resourceType string) (*rows.Observation, bool) {
	if a.store == nil {
		return nil, false
	}
	for _, sh := range a.store.Stats().Shards {
		if sh.Context == contextName && sh.Type == resourceType && sh.Namespace == "" && sh.Entries > 0 && sh.Phase == string(store.PhaseWatching) {
			return &rows.Observation{Count: sh.Entries, Bytes: sh.EstimatedBytes, ObservedAt: time.Now()}, true
		}
	}
	return nil, false
}

func (a *App) closeViews() {
	if a.viewNotifier != nil {
		a.viewNotifier.Stop()
	}
	if a.views != nil {
		a.views.Close()
	}
	if a.overlays != nil {
		a.overlays.close()
	}
}

func (a *App) resetViews() {
	if a.views != nil {
		a.views.Reset("context")
	}
	if a.overlays != nil {
		// Samples of the old cluster must not seed the views opened on the new one.
		a.overlays.hub.Reset()
		a.overlays.mu.Lock()
		a.overlays.prom = map[string]*k8s.PrometheusInfo{}
		a.overlays.mu.Unlock()
	}
}

var errViewsNotReady = errors.New("views are not ready")

func (a *App) viewManager() (*rows.ViewManager, error) {
	if a.views == nil {
		return nil, errViewsNotReady
	}
	return a.views, nil
}

// viewDisconnect closes a server-mode client's views when its WebSocket goes away. It is not an
// App method so the dispatcher does not expose it.
type viewDisconnect struct{ a *App }

func (d viewDisconnect) OnClientDisconnect(clientID string) {
	if d.a.views != nil {
		d.a.views.OnClientDisconnect(clientID)
	}
}

// =============================================================================
// View bindings. Control goes over bindings; window data goes over GET routes
// (pkg/rows/routes.go) with GetViewWindow as the binding twin of the window route.
// =============================================================================

// ViewsEnabled reports whether the view protocol is on (it is unless KUBIKLES_VIEWS=0) and
// which resource types have row specs.
type ViewsInfo struct {
	Enabled bool     `json:"enabled"`
	Types   []string `json:"types"`
	// Dynamic is true when views accept "crd:group/version/resource" types, whose row specs are
	// built from the CRD's printer columns at open time.
	Dynamic bool `json:"dynamic"`
}

// GetViewsInfo tells the frontend whether views are available and for which types. A type is
// migrated exactly when its spec is registered.
func (a *App) GetViewsInfo() ViewsInfo {
	enabled := viewsEnabled() && a.views != nil
	return ViewsInfo{Enabled: enabled, Types: rows.Types(), Dynamic: enabled}
}

// OpenView opens (or replaces) a view over a resource type.
func (a *App) OpenView(spec rows.ViewSpec) (rows.ViewOpenResult, error) {
	if !viewsEnabled() {
		return rows.ViewOpenResult{}, errors.New("views are disabled")
	}
	m, err := a.viewManager()
	if err != nil {
		return rows.ViewOpenResult{}, err
	}
	res, err := m.Open(spec)
	if err != nil {
		debug.LogPerformance("OpenView failed", map[string]interface{}{"type": spec.Type, "viewId": spec.ViewID, "error": err.Error()})
		return res, err
	}
	debug.LogPerformance("OpenView", map[string]interface{}{
		"type": spec.Type, "viewId": res.ViewID, "columns": len(res.Columns), "namespaces": len(spec.Namespaces),
		"dynamicSchema": res.Schema != nil, "phase": res.State.Phase,
	})
	return res, nil
}

// UpdateView changes a view's query (namespaces, search, filters, sort, columns).
func (a *App) UpdateView(viewId string, patch rows.ViewPatch) (rows.ViewState, error) {
	m, err := a.viewManager()
	if err != nil {
		return rows.ViewState{}, err
	}
	return m.Update(viewId, patch)
}

// CloseView closes a view. Unknown ids are ignored.
func (a *App) CloseView(viewId string) {
	if m, err := a.viewManager(); err == nil {
		m.CloseView(viewId)
		debug.LogPerformance("CloseView", map[string]interface{}{"viewId": viewId})
	}
}

// SetViewActive tells the backend whether the client currently shows the view.
func (a *App) SetViewActive(viewId string, active bool) {
	if m, err := a.viewManager(); err == nil {
		_ = m.SetActive(viewId, active)
	}
}

// GetViewWindow is the binding twin of GET /_kubikles/v1/views/{id}/window.
func (a *App) GetViewWindow(viewId string, offset int, limit int, have uint64) (rows.ViewWindow, error) {
	m, err := a.viewManager()
	if err != nil {
		return rows.ViewWindow{}, err
	}
	if limit > 1000 {
		limit = 1000
	}
	return m.Window(viewId, offset, limit, have)
}

// GetViewRefs lists row references (uid, namespace, name) for select-all and range selection.
// A limit of zero or less means to the end of the filtered list. A non-zero expectedVersion is
// the view version the offsets were taken at; the call fails with "view changed" when the list
// has moved on, so a shift-click range never selects rows the user did not see.
func (a *App) GetViewRefs(viewId string, offset int, limit int, expectedVersion uint64) ([]rows.RowRef, error) {
	m, err := a.viewManager()
	if err != nil {
		return nil, err
	}
	refs, _, err := m.Refs(viewId, offset, limit, expectedVersion)
	return refs, err
}

// GetViewFacet returns the filter values of a select column over the whole namespace scope.
func (a *App) GetViewFacet(viewId string, column string) (rows.Facet, error) {
	m, err := a.viewManager()
	if err != nil {
		return rows.Facet{}, err
	}
	return m.Facet(viewId, column)
}

// SetViewSelection tells the view which uids are selected so it can report ones that leave.
func (a *App) SetViewSelection(viewId string, uids []string) (rows.SelectionAck, error) {
	m, err := a.viewManager()
	if err != nil {
		return rows.SelectionAck{}, err
	}
	return m.SetSelection(viewId, uids)
}

// GetResourceObject returns one full object, without managed fields. A warm full-object
// shard answers from memory; otherwise it is one live GET.
func (a *App) GetResourceObject(ref rows.ObjectRef) (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), objectFetchTimeout)
	defer cancel()
	raw, err := a.getObject(ctx, ref)
	if err != nil {
		return nil, err
	}
	return decodeObject(raw)
}

// GetResourceObjects returns several objects, in the order requested.
func (a *App) GetResourceObjects(refs []rows.ObjectRef) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), objectFetchTimeout)
	defer cancel()
	out := make([]map[string]interface{}, len(refs))
	for i, ref := range refs {
		raw, err := a.getObject(ctx, ref)
		if err == nil {
			out[i], err = decodeObject(raw)
		}
		if err != nil {
			return nil, fmt.Errorf("%s %s/%s: %w", ref.Type, ref.Namespace, ref.Name, err)
		}
	}
	return out, nil
}

// decodeObject turns the JSON of an object into a map for the binding result type. Wails
// generates its TypeScript from the Go signature, and a raw JSON type has no useful shape there.
func decodeObject(raw json.RawMessage) (map[string]interface{}, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// appObjects adapts the App to rows.ObjectSource for the GET object route.
type appObjects struct{ a *App }

func (o appObjects) GetObject(ctx context.Context, ref rows.ObjectRef) (json.RawMessage, error) {
	return o.a.getObject(ctx, ref)
}

// viewRoutes is the route provider mounted into the bulk handler. The manager is resolved on
// each request because the handler exists before the store does.
func (a *App) viewRoutes() *rows.Routes {
	return &rows.Routes{Views: func() *rows.ViewManager { return a.views }, Objects: appObjects{a}}
}

func viewsEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envDisableViews))) {
	case "0", "false", "off":
		return false
	}
	return true
}

func (a *App) getObject(ctx context.Context, ref rows.ObjectRef) (json.RawMessage, error) {
	if ref.Type == "" || ref.Name == "" {
		return nil, errors.New("type and name are required")
	}
	if obj, ok := a.warmObject(ref); ok {
		return encodeObject(obj)
	}
	obj, err := a.liveObject(ctx, ref)
	if err != nil {
		return nil, err
	}
	if replacedObject(ref, obj) {
		return nil, rows.ErrObjectNotFound
	}
	return encodeObject(obj)
}

// replacedObject reports whether obj is a different object from the one ref
// names: the live fetch goes by name, so after a delete and recreate it returns
// the newcomer. The caller selected a specific object (a row), and acting on
// its replacement would be acting on something the user never chose.
func replacedObject(ref rows.ObjectRef, obj runtime.Object) bool {
	if !store.IsRealUID(ref.UID) {
		return false
	}
	acc, err := meta.Accessor(obj)
	if err != nil {
		return false
	}
	got := string(acc.GetUID())
	return got != "" && got != ref.UID
}

// warmObject looks the object up in a warm full-object shard. Table and metadata shards never
// answer, so secrets (Table in views) always take the live path.
func (a *App) warmObject(ref rows.ObjectRef) (runtime.Object, bool) {
	if a.store == nil {
		return nil, false
	}
	ctxName := a.GetCurrentContext()
	keys := []store.Key{{Context: ctxName, Type: ref.Type, Namespace: ref.Namespace}}
	if ref.Namespace != "" {
		keys = append(keys, store.Key{Context: ctxName, Type: ref.Type})
	}
	for _, k := range keys {
		if obj, _, ok := a.store.Lookup(k, ref.Namespace, ref.Name, ref.UID); ok {
			return obj, true
		}
	}
	return nil, false
}

// liveObject GETs the object from the API server: through the dynamic client where there is
// one, and through a typed list filtered by name on clients that have none (the debug cluster).
func (a *App) liveObject(ctx context.Context, ref rows.ObjectRef) (runtime.Object, error) {
	if a.k8sClient == nil {
		return nil, errNoK8sClient
	}
	ctxName := a.GetCurrentContext()
	gvr, namespaced, err := resourceFor(ref.Type)
	if err != nil {
		return nil, err
	}
	if dc, derr := a.k8sClient.DynamicClient(ctxName); derr == nil {
		var u *unstructured.Unstructured
		if namespaced && ref.Namespace != "" {
			u, err = dc.Resource(gvr).Namespace(ref.Namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		} else {
			u, err = dc.Resource(gvr).Get(ctx, ref.Name, metav1.GetOptions{})
		}
		if err != nil {
			if apierrors.IsNotFound(err) {
				return nil, rows.ErrObjectNotFound
			}
			return nil, err
		}
		return u, nil
	}
	def, ok := k8s.LookupResource(ref.Type)
	if !ok {
		return nil, fmt.Errorf("no client available for %s", ref.Type)
	}
	cs, err := a.k8sClient.KubeClient(ctxName)
	if err != nil {
		return nil, err
	}
	list, err := def.List(ctx, cs, ref.Namespace, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	items, err := meta.ExtractList(list)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if acc, aerr := meta.Accessor(it); aerr == nil && acc.GetName() == ref.Name {
			return it, nil
		}
	}
	return nil, rows.ErrObjectNotFound
}

func resourceFor(resourceType string) (schema.GroupVersionResource, bool, error) {
	if gvr, ok := store.ParseCRDType(resourceType); ok {
		return gvr, true, nil
	}
	def, ok := k8s.LookupResource(resourceType)
	if !ok {
		return schema.GroupVersionResource{}, false, fmt.Errorf("unknown resource type %q", resourceType)
	}
	return def.GVR, def.Namespaced, nil
}

// encodeObject renders an object as JSON with apiVersion and kind set and managed fields
// removed. Shared (warm) objects are never mutated: a copy gets the type meta.
func encodeObject(obj runtime.Object) (json.RawMessage, error) {
	if u, ok := obj.(*unstructured.Unstructured); ok {
		cp := u.DeepCopy()
		k8s.StripManagedFieldsObject(cp)
		return json.Marshal(cp.Object)
	}
	cp := obj.DeepCopyObject()
	k8s.StripManagedFieldsObject(cp)
	if gvks, _, err := scheme.Scheme.ObjectKinds(cp); err == nil && len(gvks) > 0 {
		cp.GetObjectKind().SetGroupVersionKind(gvks[0])
	}
	return json.Marshal(cp)
}
