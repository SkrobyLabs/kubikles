package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// TableAccept asks the API server to answer with a server-side Table, the
// representation kubectl prints. Rows carry the printer columns and a
// metadata-only object, so no spec, status or Secret data leaves the server.
const TableAccept = "application/json;as=Table;g=meta.k8s.io;v=v1"

// ErrTableUnsupported means the server (or a proxy or aggregated API in front
// of it) did not answer a Table request with a Table. Callers fall back to a
// full-object list.
var ErrTableUnsupported = errors.New("server does not support Table responses")

// TablePage is one decoded page of a Table list response.
type TablePage struct {
	Columns         []metav1.TableColumnDefinition
	Rows            []metav1.TableRow
	Continue        string
	Remaining       *int64
	ResourceVersion string
}

// TableListOptions tunes ListTable.
type TableListOptions struct {
	// PageSize is the limit parameter. Zero sends no limit at all, which keeps
	// the request eligible for the API server's watch cache.
	PageSize int64
	// RequireTable turns a response that is not a Table (an old server, a
	// proxy, an aggregated API) into ErrTableUnsupported instead of an empty
	// result.
	RequireTable bool
}

// RESTClientOf returns the REST client of a clientset, or nil when it has none
// usable. The fake clientsets used by tests and the debug cluster return a nil
// *rest.RESTClient wrapped in a non-nil interface.
func RESTClientOf(cs kubernetes.Interface) rest.Interface {
	if cs == nil {
		return nil
	}
	rc := cs.CoreV1().RESTClient()
	if rc == nil {
		return nil
	}
	if v := reflect.ValueOf(rc); v.Kind() == reflect.Ptr && v.IsNil() {
		return nil
	}
	return rc
}

// ResourcePath is the absolute API path that lists a resource, cluster wide
// when namespace is empty.
func ResourcePath(gvr schema.GroupVersionResource, namespace string) string {
	var b strings.Builder
	if gvr.Group == "" {
		b.WriteString("/api/")
	} else {
		b.WriteString("/apis/")
		b.WriteString(gvr.Group)
		b.WriteString("/")
	}
	b.WriteString(gvr.Version)
	if namespace != "" {
		b.WriteString("/namespaces/")
		b.WriteString(namespace)
	}
	b.WriteString("/")
	b.WriteString(gvr.Resource)
	return b.String()
}

// tableResponse parses a Table including the list metadata, which metav1.Table
// would otherwise shadow.
type tableResponse struct {
	metav1.Table `json:",inline"`
	Metadata     struct {
		Continue           string `json:"continue"`
		RemainingItemCount *int64 `json:"remainingItemCount,omitempty"`
		ResourceVersion    string `json:"resourceVersion,omitempty"`
	} `json:"metadata"`
}

// ListTable lists path as Tables, one page per onPage call, following continue
// tokens. A non-nil error from onPage aborts the list and is returned as is.
// Cancellation is reported as ErrRequestCancelled.
func ListTable(ctx context.Context, rc rest.Interface, path string, opts TableListOptions, onPage func(TablePage) error) error {
	if rc == nil {
		return fmt.Errorf("%w: no REST client", ErrTableUnsupported)
	}
	continueToken := ""
	for {
		if err := ctx.Err(); err != nil {
			if isCancelledError(err) {
				return ErrRequestCancelled
			}
			return err
		}

		req := rc.Get().AbsPath(path).SetHeader("Accept", TableAccept)
		// Only set limit when progress tracking is requested (avoids bypassing watch cache)
		if opts.PageSize > 0 {
			req = req.Param("limit", fmt.Sprintf("%d", opts.PageSize))
		}
		if continueToken != "" {
			req = req.Param("continue", continueToken)
		}

		result := req.Do(ctx)
		if err := result.Error(); err != nil {
			if isCancelledError(err) {
				return ErrRequestCancelled
			}
			if opts.RequireTable && (apierrors.IsNotAcceptable(err) || apierrors.IsUnsupportedMediaType(err)) {
				return fmt.Errorf("%w: %v", ErrTableUnsupported, err)
			}
			return err
		}
		body, err := result.Raw()
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}
		var table tableResponse
		if err := json.Unmarshal(body, &table); err != nil {
			return fmt.Errorf("failed to parse table response: %w", err)
		}
		if opts.RequireTable && table.Kind != "Table" {
			return fmt.Errorf("%w: response kind %q", ErrTableUnsupported, table.Kind)
		}

		page := TablePage{
			Columns:         table.ColumnDefinitions,
			Rows:            table.Rows,
			Continue:        table.Metadata.Continue,
			Remaining:       table.Metadata.RemainingItemCount,
			ResourceVersion: table.Metadata.ResourceVersion,
		}
		if err := onPage(page); err != nil {
			return err
		}
		if page.Continue == "" {
			return nil
		}
		continueToken = page.Continue
	}
}
