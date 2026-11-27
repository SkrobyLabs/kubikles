package rows

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Pieces shared by the workload specs (replica sets, daemon sets, jobs): the controller owner
// reference, pod template reads and the search fields of workloadCommonFields
// (fieldMappings/workloads.ts).

// ownerController is the first owner reference marked as controller (owners.find(o => o.controller)).
func ownerController(r *Row) *metav1.OwnerReference {
	if r.M == nil {
		return nil
	}
	refs := r.M.GetOwnerReferences()
	for i := range refs {
		if c := refs[i].Controller; c != nil && *c {
			return &refs[i]
		}
	}
	return nil
}

func controllerKind(r *Row) string {
	if c := ownerController(r); c != nil {
		return c.Kind
	}
	return ""
}

func controllerName(r *Row) string {
	if c := ownerController(r); c != nil {
		return c.Name
	}
	return ""
}

// controllerRefCell is JSON.stringify({kind, name, uid, apiVersion}) of the controller, "" without one.
func controllerRefCell(r *Row) interface{} {
	c := ownerController(r)
	if c == nil {
		return ""
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(struct {
		Kind       string `json:"kind"`
		Name       string `json:"name"`
		UID        string `json:"uid"`
		APIVersion string `json:"apiVersion"`
	}{c.Kind, c.Name, string(c.UID), c.APIVersion})
	return strings.TrimSuffix(buf.String(), "\n")
}

// labelPairs is Object.entries(labels).map(([k, v]) => `${k}=${v}`).join(sep).
func labelPairs(labels map[string]string, sep string) string {
	keys := jsKeys(labels)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + labels[k]
	}
	return strings.Join(parts, sep)
}

func selectorMatchLabels(sel *metav1.LabelSelector) map[string]string {
	if sel == nil {
		return nil
	}
	return sel.MatchLabels
}

// containerImages is the image of every container, in order.
func containerImages(cs []corev1.Container) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Image
	}
	return out
}

func containerNames(cs []corev1.Container) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}

// firstImage is containers[0].image || empty string.
func firstImage(cs []corev1.Container) string {
	if len(cs) == 0 {
		return ""
	}
	return cs[0].Image
}

// imageLines is the images one per line, the display-only cell of the Image column; "" without containers.
func imageLines(cs []corev1.Container) string { return strings.Join(containerImages(cs), "\n") }

// allContainers is [...containers, ...initContainers].
func allContainers(ps *corev1.PodSpec) []corev1.Container {
	out := make([]corev1.Container, 0, len(ps.Containers)+len(ps.InitContainers))
	out = append(out, ps.Containers...)
	return append(out, ps.InitContainers...)
}

// workloadSearchFields is workloadCommonFields minus the metadata fields: image, container,
// serviceaccount, selector, controlledby and controllername. podSpec and selector read the
// type's pod template and label selector.
func workloadSearchFields(podSpec func(*Row) *corev1.PodSpec, selector func(*Row) *metav1.LabelSelector) []SearchField {
	spec := func(r *Row) *corev1.PodSpec {
		if ps := podSpec(r); ps != nil {
			return ps
		}
		return &corev1.PodSpec{}
	}
	return []SearchField{
		{Name: "image", Aliases: []string{"images"}, Extract: func(r *Row) string {
			return strings.Join(containerImages(allContainers(spec(r))), " ")
		}},
		{Name: "container", Aliases: []string{"containers"}, Extract: func(r *Row) string {
			return strings.Join(containerNames(allContainers(spec(r))), " ")
		}},
		{Name: "serviceaccount", Aliases: []string{"sa"}, Extract: func(r *Row) string {
			ps := spec(r)
			return orDefault(ps.ServiceAccountName, ps.DeprecatedServiceAccount)
		}},
		{Name: "selector", Aliases: []string{"sel"}, Extract: func(r *Row) string {
			return labelPairs(selectorMatchLabels(selector(r)), " ")
		}},
		{Name: "controlledby", Aliases: []string{"controller", "owner"}, Extract: controllerKind},
		{Name: "controllername", Aliases: []string{"ownername"}, Extract: controllerName},
	}
}

// workloadIntColumn is a numeric count column read with `value || 0`.
func workloadIntColumn(key string, f func(r *Row) int) Column {
	return Column{Key: key, Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
		Cell: func(r *Row) interface{} { return numberCell(f(r)) }}
}

// controlledByColumns are the "Controlled By" column (the controller kind, empty without one) and the
// display-only controllerRef column the link is built from.
func controlledByColumns() []Column {
	return []Column{
		{Key: "controlledBy", Kind: KindString, CellType: "string", FilterType: FilterSelect,
			Cell: func(r *Row) interface{} { return controllerKind(r) }},
		{Key: "controllerRef", Kind: KindString, CellType: "string", FilterType: FilterNone, Cell: controllerRefCell},
	}
}

// podsColumn is the Pods column of a workload list: the number of pods the workload selects, its
// sort value (the legacy column offers no filter). It reads Row.Pods, so it is fed by Spec.Derived.
func podsColumn() Column {
	return Column{Key: "pods", Kind: KindNumber, CellType: "number", FilterType: FilterNone, Volatile: true,
		Cell: func(r *Row) interface{} { return numberCell(len(r.Pods())) }}
}

// podRefsColumn is the display-only column the squares of the Pods cell are drawn from: a JSON
// array of [name, uid, status] per selected pod, "" without any.
func podRefsColumn() Column {
	return Column{Key: "podRefs", Kind: KindString, CellType: "string", FilterType: FilterNone, Volatile: true,
		Cell: func(r *Row) interface{} { return podRefsCell(r.Pods()) }}
}

// podsDerived feeds the columns of podsColumns from the pods selected by the workload's matchLabels.
func podsDerived(selector func(*Row) *metav1.LabelSelector) Derived {
	return Derived{
		Columns:  []string{"pods", "podRefs"},
		From:     "pods",
		Selector: func(r *Row) map[string]string { return selectorMatchLabels(selector(r)) },
	}
}

func podRefsCell(pods []PodRef) interface{} {
	if len(pods) == 0 {
		return ""
	}
	refs := make([][3]string, len(pods))
	for i, p := range pods {
		refs[i] = [3]string{p.Name, p.UID, p.Status}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(refs)
	return strings.TrimSuffix(buf.String(), "\n")
}

// readyText is `${status.readyReplicas || 0}/${status.replicas || 0}`, the display of the Ready column.
func readyText(ready, total int32) string {
	return strconv.Itoa(int(ready)) + "/" + strconv.Itoa(int(total))
}
