package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/rest"

	"kubikles/pkg/k8s"
)

// tableSource lists and watches through server-side Tables. Rows carry the
// printer columns and a metadata-only object, so spec, status and Secret data
// never reach this process.
type tableSource struct {
	rc           rest.Interface
	gvr          schema.GroupVersionResource
	namespace    string
	watchTimeout int64
}

func newTableSource(rc rest.Interface, gvr schema.GroupVersionResource, namespace string, watchTimeout int64) *tableSource {
	return &tableSource{rc: rc, gvr: gvr, namespace: namespace, watchTimeout: watchTimeout}
}

func (s *tableSource) Mode() Mode { return ModeTable }

func (s *tableSource) List(ctx context.Context, opts ListOpts, onPage func(Page)) (string, error) {
	var (
		rv     string
		loaded int
	)
	err := k8s.ListTable(ctx, s.rc, k8s.ResourcePath(s.gvr, s.namespace),
		k8s.TableListOptions{PageSize: opts.PageSize, RequireTable: true},
		func(tp k8s.TablePage) error {
			if rv == "" {
				rv = tp.ResourceVersion
			}
			items := make([]runtime.Object, 0, len(tp.Rows))
			for i := range tp.Rows {
				items = append(items, rowToObject(&tp.Rows[i], tp.Columns, s.namespace))
			}
			loaded += len(items)
			onPage(Page{Items: items, Loaded: loaded, Remaining: tp.Remaining})
			return nil
		})
	if err != nil {
		return "", err
	}
	return rv, nil
}

// rowToObject converts one Table row into a TableRow. A row without an embedded
// object (includeObject=None from a proxy) is identified by its Name column.
func rowToObject(row *metav1.TableRow, cols []metav1.TableColumnDefinition, namespace string) *TableRow {
	pm := &metav1.PartialObjectMetadata{}
	if len(row.Object.Raw) > 0 {
		_ = json.Unmarshal(row.Object.Raw, pm)
	}
	if pm.Name == "" {
		for i, c := range cols {
			if c.Name == "Name" && i < len(row.Cells) {
				if n, ok := row.Cells[i].(string); ok {
					pm.Name = n
				}
			}
		}
		if pm.Namespace == "" {
			pm.Namespace = namespace
		}
	}
	return &TableRow{Meta: pm, Cells: row.Cells, Columns: cols}
}

func (s *tableSource) Watch(ctx context.Context, rv string) (watch.Interface, error) {
	timeout := s.watchTimeout
	if timeout <= 0 {
		timeout = defaultWatchTimeout
	}
	req := s.rc.Get().AbsPath(k8s.ResourcePath(s.gvr, s.namespace)).
		SetHeader("Accept", k8s.TableAccept).
		Param("watch", "true").
		Param("allowWatchBookmarks", "true").
		Param("timeoutSeconds", fmt.Sprint(timeout))
	if rv != "" {
		req = req.Param("resourceVersion", rv)
	}
	wctx, cancel := context.WithCancel(ctx)
	body, err := req.Stream(wctx)
	if err != nil {
		cancel()
		if apierrors.IsNotAcceptable(err) || apierrors.IsUnsupportedMediaType(err) {
			return nil, fmt.Errorf("%w: %v", ErrModeUnsupported, err)
		}
		return nil, err
	}
	tw := &tableWatch{body: body, cancel: cancel, ch: make(chan watch.Event, 16), ctx: wctx, namespace: s.namespace}
	go tw.run()
	return tw, nil
}

// tableWatch decodes a Table watch stream. Every event carries a Table with
// one row; columnDefinitions are only present in the first event of a stream,
// so they are remembered here. BOOKMARK and ERROR events are not Tables.
type tableWatch struct {
	body      io.ReadCloser
	cancel    context.CancelFunc
	ctx       context.Context
	ch        chan watch.Event
	namespace string
	stopOnce  sync.Once
	columns   []metav1.TableColumnDefinition
}

func (w *tableWatch) ResultChan() <-chan watch.Event { return w.ch }

func (w *tableWatch) Stop() {
	w.stopOnce.Do(func() {
		w.cancel()
		_ = w.body.Close()
	})
}

func (w *tableWatch) send(ev watch.Event) bool {
	select {
	case w.ch <- ev:
		return true
	case <-w.ctx.Done():
		return false
	}
}

type rawWatchEvent struct {
	Type   string          `json:"type"`
	Object json.RawMessage `json:"object"`
}

type objectProbe struct {
	Kind     string `json:"kind"`
	Metadata struct {
		ResourceVersion string `json:"resourceVersion"`
	} `json:"metadata"`
}

func (w *tableWatch) run() {
	defer close(w.ch)
	defer w.Stop()
	dec := json.NewDecoder(w.body)
	for {
		var raw rawWatchEvent
		if err := dec.Decode(&raw); err != nil {
			if !errors.Is(err, io.EOF) && w.ctx.Err() == nil {
				w.send(watch.Event{Type: watch.Error, Object: &metav1.Status{
					Status: metav1.StatusFailure, Message: "table watch decode: " + err.Error(),
					Reason: metav1.StatusReasonInternalError, Code: 500,
				}})
			}
			return
		}
		var probe objectProbe
		_ = json.Unmarshal(raw.Object, &probe)

		switch watch.EventType(raw.Type) {
		case watch.Error:
			st := &metav1.Status{}
			if err := json.Unmarshal(raw.Object, st); err != nil {
				st = &metav1.Status{Status: metav1.StatusFailure, Message: string(raw.Object), Reason: metav1.StatusReasonInternalError}
			}
			if !w.send(watch.Event{Type: watch.Error, Object: st}) {
				return
			}
		case watch.Bookmark:
			bm := &metav1.PartialObjectMetadata{}
			bm.ResourceVersion = probe.Metadata.ResourceVersion
			if !w.send(watch.Event{Type: watch.Bookmark, Object: bm}) {
				return
			}
		case watch.Added, watch.Modified, watch.Deleted:
			if probe.Kind != "Table" {
				// A server that ignores the Table Accept header streams full
				// objects. Report it so the shard falls back.
				w.send(watch.Event{Type: watch.Error, Object: &metav1.Status{
					Status: metav1.StatusFailure, Reason: metav1.StatusReasonNotAcceptable, Code: 406,
					Message: fmt.Sprintf("table watch returned kind %q", probe.Kind),
				}})
				return
			}
			var table metav1.Table
			if err := json.Unmarshal(raw.Object, &table); err != nil {
				w.send(watch.Event{Type: watch.Error, Object: &metav1.Status{
					Status: metav1.StatusFailure, Reason: metav1.StatusReasonInternalError,
					Message: "table watch decode: " + err.Error(), Code: 500,
				}})
				return
			}
			if len(table.ColumnDefinitions) > 0 {
				w.columns = table.ColumnDefinitions
			}
			for i := range table.Rows {
				obj := rowToObject(&table.Rows[i], w.columns, w.namespace)
				if !w.send(watch.Event{Type: watch.EventType(raw.Type), Object: obj}) {
					return
				}
			}
		default:
			// Unknown event types are ignored, as the reflector would.
		}
	}
}
