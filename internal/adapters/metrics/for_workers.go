package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type PrometheusWorkerMetrics struct {
	processedTotal  *prometheus.CounterVec
	accrualDuration *prometheus.HistogramVec
	retryAfterCount prometheus.Counter
}

func NewForWorkers(reg prometheus.Registerer) *PrometheusWorkerMetrics {
	// Бакеты для внешнего HTTP запроса (от 10мс до 10сек)
	accrualBuckets := []float64{.01, .05, .1, .25, .5, 1, 2.5, 5, 10}

	m := &PrometheusWorkerMetrics{
		processedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gmart",
			Subsystem: "accrual_worker",
			Name:      "orders_processed_total",
			Help:      "Total number of orders processed by worker with results",
		}, []string{"result"}), // success, error, no_content, 500

		accrualDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "gmart",
			Subsystem: "accrual_worker",
			Name:      "request_duration_seconds",
			Help:      "Duration of accrual service HTTP requests",
			Buckets:   accrualBuckets,
		}, []string{"code"}), // 200, 204, 429, etc.

		retryAfterCount: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "gmart",
			Subsystem: "accrual_worker",
			Name:      "rate_limit_hits_total",
			Help:      "Total number of 429 Too Many Requests received",
		}),
	}

	reg.MustRegister(m.processedTotal, m.accrualDuration, m.retryAfterCount)

	return m
}

// IncProcessed фиксирует результат обработки (success, error, timeout)
func (m *PrometheusWorkerMetrics) IncProcessed(result string) {
	m.processedTotal.WithLabelValues(result).Inc()
}

// ObserveRequest записывает время ответа сервиса начислений
func (m *PrometheusWorkerMetrics) ObserveRequest(code string, d time.Duration) {
	m.accrualDuration.WithLabelValues(code).Observe(d.Seconds())
}

// IncRateLimit фиксирует срабатывание Retry-After
func (m *PrometheusWorkerMetrics) IncRateLimit() {
	m.retryAfterCount.Inc()
}
