package rows

import (
	"strconv"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubikles/pkg/store"
)

func init() { MustRegister(jobsSpec()) }

func jobObj(r *Row) *batchv1.Job {
	o, _ := r.E.Obj.(*batchv1.Job)
	return o
}

func jobContainers(r *Row) []corev1.Container {
	if o := jobObj(r); o != nil {
		return o.Spec.Template.Spec.Containers
	}
	return nil
}

func jobPodSpec(r *Row) *corev1.PodSpec {
	if o := jobObj(r); o != nil {
		return &o.Spec.Template.Spec
	}
	return nil
}

// jobSpecInt is `value ?? fallback` for an optional spec field.
func jobSpecInt(r *Row, pick func(*batchv1.JobSpec) *int32, fallback int) int {
	if o := jobObj(r); o != nil {
		if p := pick(&o.Spec); p != nil {
			return int(*p)
		}
	}
	return fallback
}

// jobSpecOr is `value || fallback` for an optional spec field (zero and absent alike fall back).
func jobSpecOr(r *Row, pick func(*batchv1.JobSpec) *int32, fallback int) int {
	if v := jobSpecInt(r, pick, 0); v != 0 {
		return v
	}
	return fallback
}

func jobStatusInt(r *Row, pick func(*batchv1.JobStatus) int32) int {
	if o := jobObj(r); o != nil {
		return int(pick(&o.Status))
	}
	return 0
}

func jobTimeString(t *metav1.Time) string {
	if t == nil {
		return ""
	}
	return TimeString(*t)
}

// jobCompletionsText is `${status.succeeded || 0}/${spec.completions || '?'}`.
func jobCompletionsText(r *Row) string {
	completions := "?"
	if v := jobSpecInt(r, func(s *batchv1.JobSpec) *int32 { return s.Completions }, 0); v != 0 {
		completions = strconv.Itoa(v)
	}
	return strconv.Itoa(jobStatusInt(r, func(s *batchv1.JobStatus) int32 { return s.Succeeded })) + "/" + completions
}

// jobConditionText is the type of the last condition, "-" without one or with an empty type.
func jobConditionText(r *Row) string {
	if o := jobObj(r); o != nil && len(o.Status.Conditions) > 0 {
		if t := string(o.Status.Conditions[len(o.Status.Conditions)-1].Type); t != "" {
			return t
		}
	}
	return "-"
}

// jobLastConditionType is the last condition's type, "" without one (the condition search field).
func jobLastConditionType(r *Row) string {
	if o := jobObj(r); o != nil && len(o.Status.Conditions) > 0 {
		return string(o.Status.Conditions[len(o.Status.Conditions)-1].Type)
	}
	return ""
}

// jobDurationMs is the run time in milliseconds: start to completion, or to the row's evaluation
// time while the job has not completed; 0 before it started.
func jobDurationMs(r *Row) int64 {
	o := jobObj(r)
	if o == nil || o.Status.StartTime == nil {
		return 0
	}
	end := r.Now
	if o.Status.CompletionTime != nil {
		end = o.Status.CompletionTime.Time
	}
	return end.UnixMilli() - o.Status.StartTime.UnixMilli()
}

func jobsSpec() *Spec {
	completionsOf := func(s *batchv1.JobSpec) *int32 { return s.Completions }
	parallelismOf := func(s *batchv1.JobSpec) *int32 { return s.Parallelism }
	backoffOf := func(s *batchv1.JobSpec) *int32 { return s.BackoffLimit }

	// The legacy list gives these four columns no getValue: their sort key is a property the
	// object does not have, undefined for every row. The cell is always nil to match.
	undefinedColumn := func(key string, filter FilterType) Column {
		return Column{Key: key, Kind: KindString, CellType: "null", FilterType: filter,
			Cell: func(r *Row) interface{} { return nil }}
	}
	name := undefinedColumn("name", FilterRegex)
	namespace := undefinedColumn("namespace", FilterRegex)

	text := func(key string, f func(r *Row) string) Column {
		return Column{Key: key, Kind: KindString, CellType: "string", FilterType: FilterNone,
			Cell: func(r *Row) interface{} { return f(r) }}
	}
	cols := []Column{
		name,
		namespace,
		undefinedColumn("completions", FilterSelect),
		undefinedColumn("condition", FilterSelect),
		ageColumn(creationAge),
	}
	cols = append(cols, controlledByColumns()[0])
	cols = append(cols,
		workloadIntColumn("parallelism", func(r *Row) int { return jobSpecInt(r, parallelismOf, 1) }),
		workloadIntColumn("backoffLimit", func(r *Row) int { return jobSpecInt(r, backoffOf, 6) }),
		Column{Key: "activeDeadline", Kind: KindNumber, CellType: "number", FilterType: FilterSelect, Cell: func(r *Row) interface{} {
			if o := jobObj(r); o != nil && o.Spec.ActiveDeadlineSeconds != nil {
				return float64(*o.Spec.ActiveDeadlineSeconds)
			}
			return float64(0)
		}},
		Column{Key: "startTime", Kind: KindString, CellType: "string", FilterType: FilterSelect, Cell: func(r *Row) interface{} {
			if o := jobObj(r); o != nil {
				return jobTimeString(o.Status.StartTime)
			}
			return ""
		}},
		Column{Key: "duration", Kind: KindNumber, CellType: "number", FilterType: FilterSelect, Cell: func(r *Row) interface{} {
			return float64(jobDurationMs(r))
		}},
		Column{Key: "image", Kind: KindString, CellType: "string", FilterType: FilterSelect, Cell: func(r *Row) interface{} {
			return firstImage(jobContainers(r))
		}},
		// Display-only: the legacy cells read the object.
		text("completionsText", jobCompletionsText),
		text("conditionText", jobConditionText),
		text("completionTime", func(r *Row) string {
			if o := jobObj(r); o != nil {
				return jobTimeString(o.Status.CompletionTime)
			}
			return ""
		}),
		text("imageList", func(r *Row) string { return imageLines(jobContainers(r)) }),
		controlledByColumns()[1],
	)

	count := func(name string, aliases []string, f func(r *Row) int) SearchField {
		return SearchField{Name: name, Aliases: aliases, Extract: func(r *Row) string { return strconv.Itoa(f(r)) }}
	}
	search := append(commonSearchFields(),
		count("completions", nil, func(r *Row) int { return jobSpecOr(r, completionsOf, 1) }),
		count("parallelism", nil, func(r *Row) int { return jobSpecOr(r, parallelismOf, 1) }),
		count("succeeded", []string{"success"}, func(r *Row) int { return jobStatusInt(r, func(s *batchv1.JobStatus) int32 { return s.Succeeded }) }),
		count("failed", []string{"failures"}, func(r *Row) int { return jobStatusInt(r, func(s *batchv1.JobStatus) int32 { return s.Failed }) }),
		count("active", []string{"running"}, func(r *Row) int { return jobStatusInt(r, func(s *batchv1.JobStatus) int32 { return s.Active }) }),
		SearchField{Name: "condition", Aliases: []string{"status", "state"}, Extract: jobLastConditionType},
		count("backofflimit", []string{"backoff"}, func(r *Row) int { return jobSpecOr(r, backoffOf, 6) }),
	)
	// image, container, serviceaccount (no deprecated fallback), controlledby, controllername.
	wf := workloadSearchFields(jobPodSpec, func(*Row) *metav1.LabelSelector { return nil })
	search = append(search, wf[0], wf[1],
		SearchField{Name: "serviceaccount", Aliases: []string{"sa"}, Extract: func(r *Row) string {
			if ps := jobPodSpec(r); ps != nil {
				return ps.ServiceAccountName
			}
			return ""
		}},
		wf[4], wf[5])

	return &Spec{
		Type:       "jobs",
		Namespaced: true,
		Needs:      store.ModeFull, // spec, status and owner references are not metadata
		Columns:    cols,
		Search:     search,
		Notes: "The completionsText, conditionText, completionTime, imageList and controllerRef columns exist only in view mode (hidden by default): the legacy cells read the object. " +
			"The name, namespace, completions and condition columns sort and filter as they do in the legacy list, which gives them no value (their sort key is undefined for every row). " +
			"The duration of a running job is evaluated when the object last changed, not on every render: its sort order is stale between updates.",
	}
}
