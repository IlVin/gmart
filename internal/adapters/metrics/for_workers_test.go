package metrics

import (
	"bytes"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestNewForWorkers_Registration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReg := NewMockRegisterer(ctrl)

	// Ожидаем регистрацию трех объектов (processedTotal, requestDuration, rateLimitHits)
	mockReg.EXPECT().
		MustRegister(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(1)

	m := NewForWorkers(mockReg)
	assert.NotNil(t, m)
}

func TestPrometheusWorkersMetrics_Methods(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewForWorkers(reg)

	t.Run("IncProcessed", func(t *testing.T) {
		result := "success"
		m.IncProcessed(result)
		m.IncProcessed(result)

		val := testutil.ToFloat64(m.processedTotal.WithLabelValues(result))
		assert.Equal(t, 2.0, val)
	})

	t.Run("ObserveRequest", func(t *testing.T) {
		code := "200"
		duration := 500 * time.Millisecond
		m.ObserveRequest(code, duration)

		// Проверяем гистограмму через CollectAndCompare
		err := testutil.CollectAndCompare(m.requestDuration, bytes.NewBufferString(`
			# HELP gmart_worker_accrual_request_duration_seconds Duration of requests to the external Accrual Service
			# TYPE gmart_worker_accrual_request_duration_seconds histogram
			gmart_worker_accrual_request_duration_seconds_bucket{code="200",le="0.05"} 0
			gmart_worker_accrual_request_duration_seconds_bucket{code="200",le="0.1"} 0
			gmart_worker_accrual_request_duration_seconds_bucket{code="200",le="0.25"} 0
			gmart_worker_accrual_request_duration_seconds_bucket{code="200",le="0.5"} 1
			gmart_worker_accrual_request_duration_seconds_bucket{code="200",le="1"} 1
			gmart_worker_accrual_request_duration_seconds_bucket{code="200",le="2.5"} 1
			gmart_worker_accrual_request_duration_seconds_bucket{code="200",le="5"} 1
			gmart_worker_accrual_request_duration_seconds_bucket{code="200",le="10"} 1
			gmart_worker_accrual_request_duration_seconds_bucket{code="200",le="+Inf"} 1
			gmart_worker_accrual_request_duration_seconds_sum{code="200"} 0.5
			gmart_worker_accrual_request_duration_seconds_count{code="200"} 1
		`), "gmart_worker_accrual_request_duration_seconds")
		assert.NoError(t, err)
	})

	t.Run("IncRateLimit", func(t *testing.T) {
		m.IncRateLimit()
		m.IncRateLimit()
		m.IncRateLimit()

		val := testutil.ToFloat64(m.rateLimitHits)
		assert.Equal(t, 3.0, val)
	})
}
