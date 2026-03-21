package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type PrometheusWorkersMetrics struct {
	processedTotal  *prometheus.CounterVec
	requestDuration *prometheus.HistogramVec
	rateLimitHits   prometheus.Counter
}

func NewForWorkers(reg prometheus.Registerer) *PrometheusWorkersMetrics {
	// Букеты для внешних HTTP-запросов (обычно медленнее, чем БД)
	httpBuckets := []float64{.05, .1, .25, .5, 1, 2.5, 5, 10}

	m := &PrometheusWorkersMetrics{
		processedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gmart",
			Subsystem: "worker",
			Name:      "processed_items_total",
			Help:      "Total number of processed iterations by accrual worker",
		}, []string{"result"}), // success, error, timeout, no_content, rate_limit

		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "gmart",
			Subsystem: "worker",
			Name:      "accrual_request_duration_seconds",
			Help:      "Duration of requests to the external Accrual Service",
			Buckets:   httpBuckets,
		}, []string{"code"}),

		rateLimitHits: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "gmart",
			Subsystem: "worker",
			Name:      "rate_limit_hits_total",
			Help:      "Total number of 429 Too Many Requests received from external service",
		}),
	}

	reg.MustRegister(m.processedTotal, m.requestDuration, m.rateLimitHits)
	return m
}

// IncProcessed фиксирует результат итерации (success, error, timeout, no_content)
func (m *PrometheusWorkersMetrics) IncProcessed(result string) {
	m.processedTotal.WithLabelValues(result).Inc()
}

// ObserveRequest записывает длительность HTTP-запроса с кодом ответа
func (m *PrometheusWorkersMetrics) ObserveRequest(code string, duration time.Duration) {
	m.requestDuration.WithLabelValues(code).Observe(duration.Seconds())
}

// IncRateLimit инкрементирует счетчик при получении 429 ошибки
func (m *PrometheusWorkersMetrics) IncRateLimit() {
	m.rateLimitHits.Inc()
}
