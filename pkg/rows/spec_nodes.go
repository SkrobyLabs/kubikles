package rows

import (
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(nodesSpec()) }

func nodeObj(r *Row) *corev1.Node {
	o, _ := r.E.Obj.(*corev1.Node)
	return o
}

func nodeCondition(n *corev1.Node, t corev1.NodeConditionType) *corev1.NodeCondition {
	if n == nil {
		return nil
	}
	for i := range n.Status.Conditions {
		if n.Status.Conditions[i].Type == t {
			return &n.Status.Conditions[i]
		}
	}
	return nil
}

func nodeConditionTrue(n *corev1.Node, t corev1.NodeConditionType) bool {
	c := nodeCondition(n, t)
	return c != nil && c.Status == corev1.ConditionTrue
}

// nodeConditionsText is conditionsText: "Ready", or the issue types in the order getConditionsSummary
// lists them (NotReady, SchedulingDisabled, the pressure conditions, NetworkUnavailable) joined with commas.
func nodeConditionsText(n *corev1.Node) string {
	var issues []string
	if !nodeConditionTrue(n, corev1.NodeReady) {
		issues = append(issues, "NotReady")
	}
	if n != nil && n.Spec.Unschedulable {
		issues = append(issues, "SchedulingDisabled")
	}
	for _, t := range []corev1.NodeConditionType{corev1.NodeMemoryPressure, corev1.NodeDiskPressure, corev1.NodePIDPressure} {
		if nodeConditionTrue(n, t) {
			issues = append(issues, string(t))
		}
	}
	if nodeConditionTrue(n, corev1.NodeNetworkUnavailable) {
		issues = append(issues, "NetworkUnavailable")
	}
	if len(issues) == 0 {
		return "Ready"
	}
	return strings.Join(issues, ",")
}

const nodeRolePrefix = "node-role.kubernetes.io/"

// nodeRoles is the role names from the labels, in JavaScript key order.
func nodeRoles(r *Row) []string {
	if r.M == nil {
		return nil
	}
	labels := r.M.GetLabels()
	var out []string
	for _, k := range jsKeys(labels) {
		if strings.HasPrefix(k, nodeRolePrefix) {
			out = append(out, strings.TrimPrefix(k, nodeRolePrefix))
		}
	}
	return out
}

// nodeAddress is the first address of a type, "" without one.
func nodeAddress(n *corev1.Node, t corev1.NodeAddressType) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Status.Addresses {
		if a.Type == t {
			return a.Address
		}
	}
	return ""
}

func nodeString(key string, f func(n *corev1.Node) string) Column {
	return Column{Key: key, Kind: KindString, CellType: "string", FilterType: FilterSelect,
		Cell: func(r *Row) interface{} {
			if n := nodeObj(r); n != nil {
				return f(n)
			}
			return ""
		}}
}

func nodeDisplay(key string, f func(n *corev1.Node) string) Column {
	c := nodeString(key, f)
	c.FilterType = FilterNone
	return c
}

func nodeMetricColumn(key string, field int, filterable bool) Column {
	c := Column{Key: key, Kind: KindNumber, CellType: "number", FilterType: FilterSelect, Volatile: true,
		Cell: func(r *Row) interface{} { return overlayCell(r, OverlayNodeMetrics, r.Name(), field) }}
	if filterable {
		c.FilterType = FilterNumeric
		c.Numeric = func(r *Row) (float64, bool) { return overlayNumeric(r, OverlayNodeMetrics, r.Name(), field) }
	}
	return c
}

func nodesSpec() *Spec {
	cols := []Column{
		nameColumn(),
		nodeMetricColumn("cpu", nodeCPUPercent, true),
		nodeMetricColumn("memory", nodeMemPercent, true),
		// The legacy pods column has no filterType: the select default, which has nothing to list
		// because its value is a number.
		nodeMetricColumn("pods", nodePodPercent, false),
		nodeString("conditions", nodeConditionsText),
		{Key: "taints", Kind: KindNumber, CellType: "number", FilterType: FilterNone,
			Cell: func(r *Row) interface{} {
				if n := nodeObj(r); n != nil {
					return numberCell(len(n.Spec.Taints))
				}
				return numberCell(0)
			}},
		nodeString("version", func(n *corev1.Node) string { return n.Status.NodeInfo.KubeletVersion }),
		{Key: "roles", Kind: KindString, CellType: "string", FilterType: FilterSelect,
			Cell: func(r *Row) interface{} { return strings.Join(nodeRoles(r), ", ") }},
		nodeString("internalIP", func(n *corev1.Node) string { return nodeAddress(n, corev1.NodeInternalIP) }),
		nodeString("externalIP", func(n *corev1.Node) string { return nodeAddress(n, corev1.NodeExternalIP) }),
		nodeString("osImage", func(n *corev1.Node) string { return n.Status.NodeInfo.OSImage }),
		nodeString("kernelVersion", func(n *corev1.Node) string { return n.Status.NodeInfo.KernelVersion }),
		nodeString("containerRuntime", func(n *corev1.Node) string { return n.Status.NodeInfo.ContainerRuntimeVersion }),
		nodeString("architecture", func(n *corev1.Node) string { return n.Status.NodeInfo.Architecture }),
		nodeString("os", func(n *corev1.Node) string { return n.Status.NodeInfo.OperatingSystem }),
		ageColumn(creationAge),
		nodeDisplay("taintList", func(n *corev1.Node) string {
			lines := make([]string, len(n.Spec.Taints))
			for i, t := range n.Spec.Taints {
				v := ""
				if t.Value != "" {
					v = "=" + t.Value
				}
				lines[i] = t.Key + v + ":" + string(t.Effect)
			}
			return strings.Join(lines, "\n")
		}),
		nodeDisplay("unschedulable", func(n *corev1.Node) string {
			if n.Spec.Unschedulable {
				return "true"
			}
			return ""
		}),
		{Key: "metrics", Kind: KindString, CellType: "string", FilterType: FilterNone,
			Cell: func(r *Row) interface{} { return overlayRecordCell(r, OverlayNodeMetrics, r.Name()) }},
	}

	nodeField := func(name string, aliases []string, f func(n *corev1.Node) string) SearchField {
		return SearchField{Name: name, Aliases: aliases, Extract: func(r *Row) string {
			if n := nodeObj(r); n != nil {
				return f(n)
			}
			return ""
		}}
	}
	conditionStatus := func(t corev1.NodeConditionType) func(n *corev1.Node) string {
		return func(n *corev1.Node) string {
			if c := nodeCondition(n, t); c != nil {
				return string(c.Status)
			}
			return ""
		}
	}
	search := []SearchField{
		nameField(),
		labelsField(),
		annotationsField(),
		uidField(),
		nodeField("status", []string{"state"}, func(n *corev1.Node) string {
			if nodeConditionTrue(n, corev1.NodeReady) {
				return "Ready"
			}
			return "NotReady"
		}),
		{Name: "role", Aliases: []string{"roles"}, Extract: func(r *Row) string {
			if roles := nodeRoles(r); len(roles) > 0 {
				return strings.Join(roles, " ")
			}
			return "worker"
		}},
		nodeField("version", []string{"kubeletversion", "k8sversion"}, func(n *corev1.Node) string { return n.Status.NodeInfo.KubeletVersion }),
		nodeField("os", []string{"osimage"}, func(n *corev1.Node) string { return n.Status.NodeInfo.OSImage }),
		nodeField("kernel", []string{"kernelversion"}, func(n *corev1.Node) string { return n.Status.NodeInfo.KernelVersion }),
		nodeField("containerruntime", []string{"runtime", "cri"}, func(n *corev1.Node) string { return n.Status.NodeInfo.ContainerRuntimeVersion }),
		nodeField("arch", []string{"architecture"}, func(n *corev1.Node) string { return n.Status.NodeInfo.Architecture }),
		nodeField("podcidr", []string{"cidr"}, func(n *corev1.Node) string { return n.Spec.PodCIDR }),
		nodeField("internalip", []string{"ip"}, func(n *corev1.Node) string { return nodeAddress(n, corev1.NodeInternalIP) }),
		nodeField("externalip", []string{"extip"}, func(n *corev1.Node) string { return nodeAddress(n, corev1.NodeExternalIP) }),
		nodeField("hostname", []string{"host"}, func(n *corev1.Node) string { return nodeAddress(n, corev1.NodeHostName) }),
		nodeField("memorypressure", []string{"memory"}, conditionStatus(corev1.NodeMemoryPressure)),
		nodeField("diskpressure", []string{"disk"}, conditionStatus(corev1.NodeDiskPressure)),
		nodeField("pidpressure", []string{"pid"}, conditionStatus(corev1.NodePIDPressure)),
		nodeField("unschedulable", []string{"cordon", "cordoned"}, func(n *corev1.Node) string { return strconv.FormatBool(n.Spec.Unschedulable) }),
		nodeField("taints", []string{"taint"}, func(n *corev1.Node) string {
			parts := make([]string, len(n.Spec.Taints))
			for i, t := range n.Spec.Taints {
				// `${t.value}` of an absent value (the API omits empty ones) prints "undefined".
				v := t.Value
				if v == "" {
					v = "undefined"
				}
				parts[i] = t.Key + "=" + v + ":" + string(t.Effect)
			}
			return strings.Join(parts, " ")
		}),
	}
	return &Spec{
		Type:       "nodes",
		Namespaced: false,
		Needs:      store.ModeFull, // conditions, addresses, taints and node info are not metadata
		Columns:    cols,
		Search:     search,
		Overlays: []Overlay{{
			Name:    OverlayNodeMetrics,
			Columns: []string{"cpu", "memory", "pods", "metrics"},
			Key:     NodeKey,
		}},
		Notes: "The taintList, unschedulable and metrics columns exist only in view mode (hidden by default): the legacy cells read the object itself. cpu, memory and pods sort by the node metrics overlay (-1 without a sample); cpu and memory filter numerically.",
	}
}
