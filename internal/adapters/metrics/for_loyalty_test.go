package metrics

import (
	"bytes"
	"testing"
	"time"

	"gmart/internal/domain"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestNewForLoyalty_Registration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockReg := NewMockRegisterer(ctrl)

	// Ожидаем регистрацию трех объектов (dbDuration, withdrawCounts, withdrawalAmount)
	mockReg.EXPECT().
		MustRegister(gomock.Any(), gomock.Any(), gomock.Any()).
		Times(1)

	m := NewForLoyalty(mockReg)
	assert.NotNil(t, m)
}

func TestPrometheusLoyaltyMetrics_Methods(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewForLoyalty(reg)

	t.Run("ObserveDB", func(t *testing.T) {
		op := domain.OpQuery
		duration := 50 * time.Millisecond
		m.ObserveDB(op, duration)

		// Проверяем наличие записи в гистограмме (через вектор)
		count := testutil.CollectAndCount(m.dbDuration)
		assert.Equal(t, 1, count)

		err := testutil.CollectAndCompare(m.dbDuration, bytes.NewBufferString(`
			# HELP gmart_loyalty_db_operation_duration_seconds Duration of database operations in loyalty repository
			# TYPE gmart_loyalty_db_operation_duration_seconds histogram
			gmart_loyalty_db_operation_duration_seconds_bucket{op="query",le="0.001"} 0
			gmart_loyalty_db_operation_duration_seconds_bucket{op="query",le="0.005"} 0
			gmart_loyalty_db_operation_duration_seconds_bucket{op="query",le="0.01"} 0
			gmart_loyalty_db_operation_duration_seconds_bucket{op="query",le="0.025"} 0
			gmart_loyalty_db_operation_duration_seconds_bucket{op="query",le="0.05"} 1
			gmart_loyalty_db_operation_duration_seconds_bucket{op="query",le="0.1"} 1
			gmart_loyalty_db_operation_duration_seconds_bucket{op="query",le="0.25"} 1
			gmart_loyalty_db_operation_duration_seconds_bucket{op="query",le="0.5"} 1
			gmart_loyalty_db_operation_duration_seconds_bucket{op="query",le="1"} 1
			gmart_loyalty_db_operation_duration_seconds_bucket{op="query",le="2.5"} 1
			gmart_loyalty_db_operation_duration_seconds_bucket{op="query",le="5"} 1
			gmart_loyalty_db_operation_duration_seconds_bucket{op="query",le="+Inf"} 1
			gmart_loyalty_db_operation_duration_seconds_sum{op="query"} 0.05
			gmart_loyalty_db_operation_duration_seconds_count{op="query"} 1
		`), "gmart_loyalty_db_operation_duration_seconds")
		assert.NoError(t, err)
	})

	t.Run("IncWithdrawal", func(t *testing.T) {
		status := "processed"
		m.IncWithdrawal(status)
		m.IncWithdrawal(status)

		val := testutil.ToFloat64(m.withdrawCounts.WithLabelValues(status))
		assert.Equal(t, 2.0, val)
	})

	t.Run("ObserveWithdrawalAmount", func(t *testing.T) {
		amount := domain.Amount(5500) // 55 рублей
		m.ObserveWithdrawalAmount(amount)

		// withdrawalAmount — это Histogram (не Vec), проверяем напрямую
		err := testutil.CollectAndCompare(m.withdrawalAmount, bytes.NewBufferString(`
			# HELP gmart_loyalty_withdrawal_amount_cents Distribution of withdrawal amounts in cents
			# TYPE gmart_loyalty_withdrawal_amount_cents histogram
			gmart_loyalty_withdrawal_amount_cents_bucket{le="1000"} 0
			gmart_loyalty_withdrawal_amount_cents_bucket{le="5000"} 0
			gmart_loyalty_withdrawal_amount_cents_bucket{le="10000"} 1
			gmart_loyalty_withdrawal_amount_cents_bucket{le="50000"} 1
			gmart_loyalty_withdrawal_amount_cents_bucket{le="100000"} 1
			gmart_loyalty_withdrawal_amount_cents_bucket{le="500000"} 1
			gmart_loyalty_withdrawal_amount_cents_bucket{le="+Inf"} 1
			gmart_loyalty_withdrawal_amount_cents_sum 5500
			gmart_loyalty_withdrawal_amount_cents_count 1
		`), "gmart_loyalty_withdrawal_amount_cents")
		assert.NoError(t, err)
	})
}
