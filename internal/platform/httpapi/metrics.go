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
	buildVersion string
	startedAt    time.Time
	requests     atomic.Uint64
	responses    [6]atomic.Uint64
}

func newHTTPMetrics(buildVersion string) *httpMetrics {
	return &httpMetrics{buildVersion: buildVersion, startedAt: time.Now()}
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
}

func prometheusLabel(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\"", "\\\"")
	return replacer.Replace(value)
}
