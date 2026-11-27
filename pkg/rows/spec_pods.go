package rows

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(podsSpec()) }

func podObj(r *Row) *corev1.Pod {
	o, _ := r.E.Obj.(*corev1.Pod)
	return o
}

// podStatus is getPodStatus: Terminating, then the first init container problem, then the phase.
// The init container check mirrors the legacy one: an init container without a running or
// terminated state (waiting or unset) counts as still initialising.
func podStatus(p *corev1.Pod) string {
	if p == nil {
		return "Unknown"
	}
	if p.DeletionTimestamp != nil {
		return "Terminating"
	}
	for i := range p.Status.InitContainerStatuses {
		st := &p.Status.InitContainerStatuses[i].State
		if st.Terminated != nil && st.Terminated.ExitCode != 0 {
			return "Init:Error"
		}
		if st.Waiting != nil && st.Waiting.Reason == "CrashLoopBackOff" {
			return "Init:CrashLoopBackOff"
		}
		if st.Running == nil && st.Terminated == nil {
			return "Init:Running"
		}
	}
	return orDefault(string(p.Status.Phase), "Unknown")
}

// podStatusPriority is getPodStatusPriority.
func podStatusPriority(status string) float64 {
	switch status {
	case "Failed", "Init:Error":
		return 1
	case "CrashLoopBackOff", "Init:CrashLoopBackOff":
		return 2
	case "ErrImagePull", "Init:ErrImagePull":
		return 3
	case "ImagePullBackOff", "Init:ImagePullBackOff":
		return 4
	case "Terminating":
		return 5
	case "Pending", "Init:Running", "Init:Waiting", "PodInitializing":
		return 6
	case "ContainerCreating":
		return 7
	case "Running":
		return 8
	case "Succeeded":
		return 9
	}
	return 10
}

// podSearchStatus is getPodStatusForSearch: like podStatus, then the first container waiting for
// a reason worth naming, then the phase.
func podSearchStatus(p *corev1.Pod) string {
	if p == nil {
		return "Unknown"
	}
	if p.DeletionTimestamp != nil {
		return "Terminating"
	}
	for i := range p.Status.InitContainerStatuses {
		st := &p.Status.InitContainerStatuses[i].State
		if st.Terminated != nil && st.Terminated.ExitCode != 0 {
			return "Init:Error"
		}
		if st.Waiting != nil && st.Waiting.Reason == "CrashLoopBackOff" {
			return "Init:CrashLoopBackOff"
		}
		if st.Running == nil && st.Terminated == nil {
			return "Init:Running"
		}
	}
	for i := range p.Status.ContainerStatuses {
		if w := p.Status.ContainerStatuses[i].State.Waiting; w != nil {
			switch w.Reason {
			case "CrashLoopBackOff", "ErrImagePull", "ImagePullBackOff":
				return w.Reason
			}
		}
	}
	return orDefault(string(p.Status.Phase), "Unknown")
}

// podRestarts is the sum of the container restart counts.
func podRestarts(p *corev1.Pod) int {
	if p == nil {
		return 0
	}
	n := 0
	for i := range p.Status.ContainerStatuses {
		n += int(p.Status.ContainerStatuses[i].RestartCount)
	}
	return n
}

// podContainerStatesCell is podContainerStatesText: the container statuses as compact JSON
// ([{"n":name,"s":state kind,"w":waiting reason,"e":exit code}]), the data behind the squares.
func podContainerStatesCell(p *corev1.Pod) string {
	type cell struct {
		N string `json:"n"`
		S string `json:"s"`
		W string `json:"w"`
		E *int32 `json:"e"`
	}
	cells := make([]cell, 0)
	if p != nil {
		for i := range p.Status.ContainerStatuses {
			cs := &p.Status.ContainerStatuses[i]
			c := cell{N: cs.Name}
			// Object.keys of the serialised state lists waiting, running, terminated in that order.
			switch {
			case cs.State.Waiting != nil:
				c.S = "waiting"
			case cs.State.Running != nil:
				c.S = "running"
			case cs.State.Terminated != nil:
				c.S = "terminated"
			}
			if cs.State.Waiting != nil {
				c.W = cs.State.Waiting.Reason
			}
			if cs.State.Terminated != nil {
				e := cs.State.Terminated.ExitCode
				c.E = &e
			}
			cells = append(cells, c)
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(cells)
	return strings.TrimSuffix(buf.String(), "\n")
}

func podStringColumn(key string, f func(p *corev1.Pod) string) Column {
	return Column{Key: key, Kind: KindString, CellType: "string", FilterType: FilterSelect,
		Cell: func(r *Row) interface{} {
			if p := podObj(r); p != nil {
				return f(p)
			}
			return ""
		}}
}

func podDisplayColumn(key string, f func(p *corev1.Pod) string) Column {
	c := podStringColumn(key, f)
	c.FilterType = FilterNone
	return c
}

func podKey(r *Row) string { return PodKey(r.Namespace(), r.Name()) }

// podMetricColumn is the cpu or memory column: sorted by the committed percentage (-1 without a
// sample) and filtered by the usage percentage (NaN without one).
func podMetricColumn(key string, sortField, filterField int) Column {
	return Column{Key: key, Kind: KindNumber, CellType: "number", FilterType: FilterNumeric, Volatile: true,
		Cell: func(r *Row) interface{} { return overlayCell(r, OverlayPodMetrics, podKey(r), sortField) },
		Numeric: func(r *Row) (float64, bool) {
			return overlayNumeric(r, OverlayPodMetrics, podKey(r), filterField)
		}}
}

func podsSpec() *Spec {
	cols := []Column{
		nameColumn(),
		namespaceColumn(),
		podMetricColumn("cpu", podCPUCommittedPercent, podCPUPercent),
		podMetricColumn("memory", podMemCommittedPercent, podMemPercent),
		{Key: "containers", Kind: KindNumber, CellType: "number", FilterType: FilterNone,
			Cell: func(r *Row) interface{} { return podStatusPriority(podStatus(podObj(r))) }},
		{Key: "status", Kind: KindNumber, CellType: "number", FilterType: FilterSelect,
			Predefined: []string{
				"Running", "Pending", "Failed", "Succeeded", "CrashLoopBackOff", "Terminating",
				"Unknown", "ErrImagePull", "ImagePullBackOff", "ContainerCreating", "PodInitializing",
				"Init:Error", "Init:CrashLoopBackOff", "Init:Running",
			},
			Cell:   func(r *Row) interface{} { return podStatusPriority(podStatus(podObj(r))) },
			Filter: func(r *Row) string { return podStatus(podObj(r)) }},
		{Key: "restarts", Kind: KindNumber, CellType: "number", FilterType: FilterNumeric,
			Cell: func(r *Row) interface{} { return numberCell(podRestarts(podObj(r))) }},
		ageColumn(creationAge),
		{Key: "controlledBy", Kind: KindString, CellType: "string", FilterType: FilterSelect,
			Cell: func(r *Row) interface{} { return controllerKind(r) }},
		podStringColumn("qos", func(p *corev1.Pod) string { return string(p.Status.QOSClass) }),
		podStringColumn("node", func(p *corev1.Pod) string { return p.Spec.NodeName }),
		podStringColumn("podIP", func(p *corev1.Pod) string { return p.Status.PodIP }),
		podStringColumn("hostIP", func(p *corev1.Pod) string { return p.Status.HostIP }),
		podStringColumn("serviceAccount", func(p *corev1.Pod) string { return orDefault(p.Spec.ServiceAccountName, "default") }),
		podStringColumn("priorityClass", func(p *corev1.Pod) string { return p.Spec.PriorityClassName }),
		podStringColumn("image", func(p *corev1.Pod) string { return firstImage(p.Spec.Containers) }),
		// Display-only columns: the legacy cells read the object (status text, container squares,
		// controller link, image list, eviction check, metrics sample).
		podDisplayColumn("statusText", func(p *corev1.Pod) string { return podStatus(p) }),
		podDisplayColumn("phase", func(p *corev1.Pod) string { return string(p.Status.Phase) }),
		podDisplayColumn("containerStates", podContainerStatesCell),
		{Key: "controllerRef", Kind: KindString, CellType: "string", FilterType: FilterNone, Cell: controllerRefCell},
		podDisplayColumn("imageList", func(p *corev1.Pod) string { return imageLines(p.Spec.Containers) }),
		{Key: "metrics", Kind: KindString, CellType: "string", FilterType: FilterNone,
			Cell: func(r *Row) interface{} { return overlayRecordCell(r, OverlayPodMetrics, podKey(r)) }},
	}

	podSpec := func(r *Row) *corev1.PodSpec {
		if p := podObj(r); p != nil {
			return &p.Spec
		}
		return &corev1.PodSpec{}
	}
	podStatusOf := func(r *Row) *corev1.PodStatus {
		if p := podObj(r); p != nil {
			return &p.Status
		}
		return &corev1.PodStatus{}
	}
	search := append(commonSearchFields(),
		SearchField{Name: "nodename", Aliases: []string{"node"}, Extract: func(r *Row) string { return podSpec(r).NodeName }},
		SearchField{Name: "status", Aliases: []string{"phase", "s"}, Extract: func(r *Row) string { return podSearchStatus(podObj(r)) }},
		SearchField{Name: "ip", Aliases: []string{"podip"}, Extract: func(r *Row) string { return podStatusOf(r).PodIP }},
		SearchField{Name: "hostip", Extract: func(r *Row) string { return podStatusOf(r).HostIP }},
		SearchField{Name: "restarts", Aliases: []string{"restart"}, Extract: func(r *Row) string { return strconv.Itoa(podRestarts(podObj(r))) }},
		SearchField{Name: "controlledby", Aliases: []string{"controller", "owner"}, Extract: controllerKind},
		SearchField{Name: "controllername", Aliases: []string{"ownername"}, Extract: controllerName},
		SearchField{Name: "container", Aliases: []string{"containers"}, Extract: func(r *Row) string {
			return strings.Join(containerNames(allContainers(podSpec(r))), " ")
		}},
		SearchField{Name: "image", Aliases: []string{"images"}, Extract: func(r *Row) string {
			return strings.Join(containerImages(allContainers(podSpec(r))), " ")
		}},
		SearchField{Name: "serviceaccount", Aliases: []string{"sa"}, Extract: func(r *Row) string {
			ps := podSpec(r)
			return orDefault(ps.ServiceAccountName, ps.DeprecatedServiceAccount)
		}},
		SearchField{Name: "qos", Aliases: []string{"qosclass"}, Extract: func(r *Row) string { return string(podStatusOf(r).QOSClass) }},
	)
	return &Spec{
		Type:       "pods",
		Namespaced: true,
		Needs:      store.ModeFull, // status, container statuses and the pod spec are not metadata
		Columns:    cols,
		Search:     search,
		Overlays: []Overlay{{
			Name:    OverlayPodMetrics,
			Columns: []string{"cpu", "memory", "metrics"},
			Key:     PodKey,
		}},
		Notes: "The statusText, phase, containerStates, controllerRef, imageList and metrics columns exist only in view mode (hidden by default): the legacy cells read the object itself. cpu and memory sort and filter by the metrics overlay; the metrics cell carries the sample as comma-joined numbers (metricsRecordText).",
	}
}
