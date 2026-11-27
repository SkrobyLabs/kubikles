package k8s

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"
)

// prometheusQueryConcurrency bounds how many range queries one history call
// runs at once. History views issue 10 to 25 independent queries; running them
// one after another made the panel as slow as the sum of every query. A small
// bound keeps the load on a (often small) in-cluster Prometheus modest and
// stays well inside the API server proxy's per-client limits.
const prometheusQueryConcurrency = 6

// rangeQueryBatch runs independent best-effort range queries concurrently.
// Each query writes to its own destination, so results land exactly where the
// sequential code put them and no ordering is lost. Failures stay silent per
// query (queryRangeToDataPointsWithContext logs and returns an empty series),
// as before. Call wait before reading any destination.
type rangeQueryBatch struct {
	c           *Client
	ctx         context.Context
	contextName string
	info        *PrometheusInfo
	start, end  time.Time
	step        time.Duration
	g           errgroup.Group
}

func (c *Client) newRangeQueryBatch(ctx context.Context, contextName string, info *PrometheusInfo, start, end time.Time, step time.Duration) *rangeQueryBatch {
	b := &rangeQueryBatch{c: c, ctx: ctx, contextName: contextName, info: info, start: start, end: end, step: step}
	b.g.SetLimit(prometheusQueryConcurrency)
	return b
}

// points queues query and stores its first series into *dst.
func (b *rangeQueryBatch) points(dst *[]MetricsDataPoint, query string) {
	b.g.Go(func() error {
		*dst = b.c.queryRangeToDataPointsWithContext(b.ctx, b.contextName, b.info, query, b.start, b.end, b.step)
		return nil
	})
}

// contributors queues a node memory contributor query.
func (b *rangeQueryBatch) contributors(dst *[]NodeMemoryContributor, query string) {
	b.g.Go(func() error {
		*dst = b.c.queryNodeMemoryContributorsWithContext(b.ctx, b.contextName, b.info, query, b.start, b.end, b.step)
		return nil
	})
}

// wait blocks until every queued query has finished.
func (b *rangeQueryBatch) wait() { _ = b.g.Wait() }
