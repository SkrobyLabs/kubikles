// Code split from prometheus.go; see that file for the Prometheus types and detection.
package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"kubikles/pkg/debug"
)

// queryPrometheusRaw makes a raw query to Prometheus via K8s API proxy
func (c *Client) queryPrometheusRaw(contextName string, info *PrometheusInfo, path string, params map[string]string) ([]byte, error) {
	return c.queryPrometheusRawWithContext(context.Background(), contextName, info, path, params)
}

// queryPrometheusRawWithContext makes a raw query to Prometheus with cancellation support.
// Uses POST with form-encoded body for parameterized queries to avoid URL-length limits
// that can occur with complex PromQL queries through the K8s API proxy.
// Uses Do() instead of DoRaw() to preserve response body on error for diagnostics.
func (c *Client) queryPrometheusRawWithContext(ctx context.Context, contextName string, info *PrometheusInfo, path string, params map[string]string) ([]byte, error) {
	cs, err := c.getClientsetForContext(contextName)
	if err != nil {
		return nil, err
	}

	svcName := fmt.Sprintf("%s:%d", info.Service, info.Port)

	// Use POST with form-encoded body when params are present (query_range, query).
	// GET URL params can hit K8s API proxy URL-length limits with complex PromQL.
	if len(params) > 0 {
		formData := url.Values{}
		for k, v := range params {
			formData.Set(k, v)
		}

		req := cs.CoreV1().RESTClient().Post().
			Namespace(info.Namespace).
			Resource("services").
			Name(svcName).
			SubResource("proxy").
			Suffix(path).
			SetHeader("Content-Type", "application/x-www-form-urlencoded").
			Body([]byte(formData.Encode()))

		// Use Do() instead of DoRaw() — Do() preserves the response body even on
		// non-2xx status, letting us capture the actual Prometheus error message.
		rawResult := req.Do(ctx)
		var statusCode int
		rawResult.StatusCode(&statusCode)
		body, err := rawResult.Raw()
		if err != nil {
			promErr := extractPrometheusError(body)
			debug.LogK8s("Prometheus query failed", map[string]interface{}{
				"error":      err.Error(),
				"httpStatus": statusCode,
				"promError":  promErr,
				"path":       path,
				"service":    fmt.Sprintf("%s/%s", info.Namespace, svcName),
				"query":      params["query"],
				"start":      params["start"],
				"end":        params["end"],
				"step":       params["step"],
			})
			if promErr != "" {
				return nil, fmt.Errorf("prometheus error: %s", promErr)
			}
			return nil, fmt.Errorf("prometheus query failed: %w", err)
		}
		return body, nil
	}

	// GET for simple requests without params (e.g. connection tests)
	req := cs.CoreV1().RESTClient().Get().
		Namespace(info.Namespace).
		Resource("services").
		Name(svcName).
		SubResource("proxy").
		Suffix(path)

	result, err := req.DoRaw(ctx)
	if err != nil {
		return nil, fmt.Errorf("prometheus query failed: %w", err)
	}

	return result, nil
}

// QueryPrometheus executes an instant query against Prometheus
func (c *Client) QueryPrometheus(contextName string, info *PrometheusInfo, query string) (*PrometheusQueryResult, error) {
	return c.QueryPrometheusWithContext(context.Background(), contextName, info, query)
}

// QueryPrometheusWithContext executes an instant query against Prometheus with cancellation support
func (c *Client) QueryPrometheusWithContext(ctx context.Context, contextName string, info *PrometheusInfo, query string) (*PrometheusQueryResult, error) {
	params := map[string]string{
		"query": query,
	}

	data, err := c.queryPrometheusRawWithContext(ctx, contextName, info, "api/v1/query", params)
	if err != nil {
		return nil, err
	}

	var result PrometheusQueryResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse prometheus response: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("prometheus query error: %s - %s", result.ErrorType, result.Error)
	}

	return &result, nil
}

// QueryPrometheusRangeWithContext executes a range query against Prometheus with cancellation support
func (c *Client) QueryPrometheusRangeWithContext(ctx context.Context, contextName string, info *PrometheusInfo, query string, start, end time.Time, step time.Duration) (*PrometheusQueryResult, error) {
	params := map[string]string{
		"query": query,
		"start": fmt.Sprintf("%d", start.Unix()),
		"end":   fmt.Sprintf("%d", end.Unix()),
		"step":  fmt.Sprintf("%d", int(step.Seconds())),
	}

	data, err := c.queryPrometheusRawWithContext(ctx, contextName, info, "api/v1/query_range", params)
	if err != nil {
		return nil, err
	}

	var result PrometheusQueryResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse prometheus response: %w", err)
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("prometheus query error: %s - %s", result.ErrorType, result.Error)
	}

	return &result, nil
}

// calculateMetricsStep computes a rounded Prometheus step interval for the given time range and target data points.
func calculateMetricsStep(start, end time.Time, maxDataPoints int) time.Duration {
	duration := end.Sub(start)
	step := duration / time.Duration(maxDataPoints)
	if step < 15*time.Second {
		step = 15 * time.Second
	}
	switch {
	case step < 30*time.Second:
		step = 15 * time.Second
	case step < time.Minute:
		step = 30 * time.Second
	case step < 5*time.Minute:
		step = time.Minute
	case step < 15*time.Minute:
		step = 5 * time.Minute
	case step < 30*time.Minute:
		step = 15 * time.Minute
	case step < time.Hour:
		step = 30 * time.Minute
	default:
		step = time.Hour
	}
	return step
}
