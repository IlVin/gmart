package metrics

import (
	"time"

	"gmart/internal/domain"

	"github.com/prometheus/client_golang/prometheus"
)

type PrometheusOrdersMetrics struct {
	dbDuration      *prometheus.HistogramVec
	uploadCounts    *prometheus.CounterVec
	acquireAttempts *prometheus.CounterVec
	listSize        prometheus.Histogram
	finalizedCounts *prometheus.CounterVec
}

// Для OrdersRepo & WorkersRepo
func NewForOrders(reg prometheus.Registerer) *PrometheusOrdersMetrics {
	dbBuckets := []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5}
	sizeBuckets := []float64{0, 1, 5, 10, 20, 50, 100}

	m := &PrometheusOrdersMetrics{
		dbDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "gmart",
			Subsystem: "orders",
			Name:      "db_operation_duration_seconds",
			Help:      "Duration of database operations",
			Buckets:   dbBuckets,
		}, []string{"op"}),

		uploadCounts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gmart",
			Subsystem: "orders",
			Name:      "upload_total",
			Help:      "Total number of order upload attempts",
		}, []string{"result"}),

		acquireAttempts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gmart",
			Subsystem: "orders",
			Name:      "acquire_attempts_total",
			Help:      "Total number of worker acquire attempts",
		}, []string{"found"}),

		listSize: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "gmart",
			Subsystem: "orders",
			Name:      "list_size_rows",
			Help:      "Number of orders returned in a single list request",
			Buckets:   sizeBuckets,
		}),

		// Реализация для воркеров: считаем сколько заказов перешло в конечный статус
		finalizedCounts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "gmart",
			Subsystem: "orders",
			Name:      "finalized_total",
			Help:      "Total number of orders reached terminal status (PROCESSED/INVALID)",
		}, []string{"status"}),
	}

	reg.MustRegister(m.dbDuration, m.uploadCounts, m.acquireAttempts, m.listSize, m.finalizedCounts)
	return m
}

// ObserveDB записывает время выполнения запроса к БД (Удовлетворяет обоим интерфейсам)
func (m *PrometheusOrdersMetrics) ObserveDB(op OpType, d time.Duration) {
	m.dbDuration.WithLabelValues(op.String()).Observe(d.Seconds())
}

// IncOrderUpload (Для OrdersRepo)
func (m *PrometheusOrdersMetrics) IncOrderUpload(result string) {
	m.uploadCounts.WithLabelValues(result).Inc()
}

// IncAcquireAttempt (Для WorkersRepo)
func (m *PrometheusOrdersMetrics) IncAcquireAttempt(found bool) {
	label := "false"
	if found {
		label = "true"
	}
	m.acquireAttempts.WithLabelValues(label).Inc()
}

// ObserveListSize записывает количество найденных заказов (Удовлетворяет обоим интерфейсам)
func (m *PrometheusOrdersMetrics) ObserveListSize(size int) {
	m.listSize.Observe(float64(size))
}

// IncOrderFinalized записывает переход заказа в финальное состояние (Для WorkersRepo)
func (m *PrometheusOrdersMetrics) IncOrderFinalized(status domain.OrderStatus) {
	m.finalizedCounts.WithLabelValues(string(status)).Inc()
}
