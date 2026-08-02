package httpapi

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

type httpMetrics struct {
	buildVersion      string
	startedAt         time.Time
	runtimeLogDropped func() uint64
	runtimeLogQueued  func() int
	runtimeLogBytes   func() int64
	databasePools     []DatabasePoolMetric
	requests          atomic.Uint64
	responses         [6]atomic.Uint64
}

func newHTTPMetrics(buildVersion string, runtimeLogDropped func() uint64, runtimeLogQueued func() int, runtimeLogBytes func() int64, databasePools []DatabasePoolMetric) *httpMetrics {
	return &httpMetrics{
		buildVersion: buildVersion, startedAt: time.Now(),
		runtimeLogDropped: runtimeLogDropped, runtimeLogQueued: runtimeLogQueued,
		runtimeLogBytes: runtimeLogBytes, databasePools: append([]DatabasePoolMetric(nil), databasePools...),
	}
}

func (metrics *httpMetrics) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		metrics.requests.Add(1)
		wrapped := chimiddleware.NewWrapResponseWriter(writer, request.ProtoMajor)
		next.ServeHTTP(wrapped, request)
		status := wrapped.Status()
		if status == 0 {
			status = http.StatusOK
		}
		statusClass := status / 100
		if statusClass >= 1 && statusClass <= 5 {
			metrics.responses[statusClass].Add(1)
		}
	})
}

func (metrics *httpMetrics) serveHTTP(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintf(writer, "# HELP werk_build_info WERK API build information.\n")
	_, _ = fmt.Fprintf(writer, "# TYPE werk_build_info gauge\n")
	_, _ = fmt.Fprintf(writer, "werk_build_info{service=\"werk-api\",version=\"%s\"} 1\n", prometheusLabel(metrics.buildVersion))
	_, _ = fmt.Fprintf(writer, "# HELP werk_process_uptime_seconds Seconds since the API process created its router.\n")
	_, _ = fmt.Fprintf(writer, "# TYPE werk_process_uptime_seconds gauge\n")
	_, _ = fmt.Fprintf(writer, "werk_process_uptime_seconds %d\n", int64(time.Since(metrics.startedAt).Seconds()))
	_, _ = fmt.Fprintf(writer, "# HELP werk_http_requests_total HTTP requests received.\n")
	_, _ = fmt.Fprintf(writer, "# TYPE werk_http_requests_total counter\n")
	_, _ = fmt.Fprintf(writer, "werk_http_requests_total %d\n", metrics.requests.Load())
	_, _ = fmt.Fprintf(writer, "# HELP werk_http_responses_total HTTP responses grouped by status class.\n")
	_, _ = fmt.Fprintf(writer, "# TYPE werk_http_responses_total counter\n")
	for statusClass := 1; statusClass <= 5; statusClass++ {
		_, _ = fmt.Fprintf(writer, "werk_http_responses_total{status_class=\"%dxx\"} %d\n", statusClass, metrics.responses[statusClass].Load())
	}
	dropped := uint64(0)
	if metrics.runtimeLogDropped != nil {
		dropped = metrics.runtimeLogDropped()
	}
	_, _ = fmt.Fprintf(writer, "# HELP werk_kafka_runtime_logs_dropped_total Runtime log records not exported to Kafka.\n")
	_, _ = fmt.Fprintf(writer, "# TYPE werk_kafka_runtime_logs_dropped_total counter\n")
	_, _ = fmt.Fprintf(writer, "werk_kafka_runtime_logs_dropped_total %d\n", dropped)
	queued := 0
	if metrics.runtimeLogQueued != nil {
		queued = metrics.runtimeLogQueued()
	}
	queuedBytes := int64(0)
	if metrics.runtimeLogBytes != nil {
		queuedBytes = metrics.runtimeLogBytes()
	}
	_, _ = fmt.Fprintf(writer, "# HELP werk_kafka_runtime_log_queue_entries Runtime log records waiting for or currently in Kafka publication.\n")
	_, _ = fmt.Fprintf(writer, "# TYPE werk_kafka_runtime_log_queue_entries gauge\n")
	_, _ = fmt.Fprintf(writer, "werk_kafka_runtime_log_queue_entries %d\n", queued)
	_, _ = fmt.Fprintf(writer, "# HELP werk_kafka_runtime_log_queue_bytes Encoded runtime log bytes waiting for or currently in Kafka publication.\n")
	_, _ = fmt.Fprintf(writer, "# TYPE werk_kafka_runtime_log_queue_bytes gauge\n")
	_, _ = fmt.Fprintf(writer, "werk_kafka_runtime_log_queue_bytes %d\n", queuedBytes)
	metrics.writeDatabasePools(writer)
}

func (metrics *httpMetrics) writeDatabasePools(writer http.ResponseWriter) {
	_, _ = fmt.Fprintln(writer, "# HELP werk_database_pool_connections PostgreSQL pool connections by fixed API role and state.")
	_, _ = fmt.Fprintln(writer, "# TYPE werk_database_pool_connections gauge")
	_, _ = fmt.Fprintln(writer, "# HELP werk_database_pool_acquire_wait_total Number of acquisitions that waited for a PostgreSQL connection.")
	_, _ = fmt.Fprintln(writer, "# TYPE werk_database_pool_acquire_wait_total counter")
	_, _ = fmt.Fprintln(writer, "# HELP werk_database_pool_acquire_wait_seconds_total Cumulative time waiting for PostgreSQL connections.")
	_, _ = fmt.Fprintln(writer, "# TYPE werk_database_pool_acquire_wait_seconds_total counter")
	for _, pool := range metrics.databasePools {
		if pool.Snapshot == nil || (pool.Name != "work" && pool.Name != "identity" && pool.Name != "admin") {
			continue
		}
		stats := pool.Snapshot()
		for state, value := range map[string]int32{"max": stats.Max, "total": stats.Total, "acquired": stats.Acquired, "idle": stats.Idle, "constructing": stats.Constructing} {
			_, _ = fmt.Fprintf(writer, "werk_database_pool_connections{pool=\"%s\",state=\"%s\"} %d\n", pool.Name, state, value)
		}
		_, _ = fmt.Fprintf(writer, "werk_database_pool_acquire_wait_total{pool=\"%s\"} %d\n", pool.Name, stats.AcquireWaits)
		_, _ = fmt.Fprintf(writer, "werk_database_pool_acquire_wait_seconds_total{pool=\"%s\"} %f\n", pool.Name, stats.AcquireTime.Seconds())
	}
}

func prometheusLabel(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\"", "\\\"")
	return replacer.Replace(value)
}
