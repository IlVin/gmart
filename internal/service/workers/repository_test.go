package workers

import (
	"context"
	"testing"
	"time"

	"gmart/internal/adapters/metrics"
	"gmart/internal/adapters/pgc"
	"gmart/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestWorkersRepo_AcquireNextOrder(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetrics := NewMockWorkersMetricsRepoIFace(ctrl)
	mockPool := NewMockPgxPoolIface(ctrl)

	// Настройка фейкового инстанса БД, который пробрасывает коллбэк в мок пула
	mockPg := NewMockPgInstance(ctrl)
	mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, fn func(context.Context, pgc.PgxPoolIface) error) error {
			return fn(ctx, mockPool)
		}).AnyTimes()

	fixedTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	repo := &WorkersRepo{
		pg:      mockPg,
		metrics: mockMetrics,
		now:     func() time.Time { return fixedTime },
	}

	t.Run("success_acquire", func(t *testing.T) {
		orderNum := domain.OrderNumber("12345")
		status := domain.OrderStatus("NEW")

		mockRow := NewMockRow(ctrl) // Нужен мок для pgx.Row
		mockPool.EXPECT().QueryRow(gomock.Any(), sqlAcquireNextOrder, fixedTime).Return(mockRow)
		mockRow.EXPECT().Scan(gomock.Any(), gomock.Any()).DoAndReturn(func(dest ...interface{}) error {
			*(dest[0].(*domain.OrderNumber)) = orderNum
			*(dest[1].(*domain.OrderStatus)) = status
			return nil
		})

		mockMetrics.EXPECT().ObserveDB(metrics.OpQuery, gomock.Any())
		mockMetrics.EXPECT().IncAcquireAttempt(true)

		resNum, resStatus, err := repo.AcquireNextOrder(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, orderNum, resNum)
		assert.Equal(t, status, resStatus)
	})

	t.Run("queue_empty", func(t *testing.T) {
		mockRow := NewMockRow(ctrl)
		mockPool.EXPECT().QueryRow(gomock.Any(), sqlAcquireNextOrder, fixedTime).Return(mockRow)
		mockRow.EXPECT().Scan(gomock.Any(), gomock.Any()).Return(pgx.ErrNoRows)

		mockMetrics.EXPECT().ObserveDB(metrics.OpQuery, gomock.Any())
		mockMetrics.EXPECT().IncAcquireAttempt(false)

		_, _, err := repo.AcquireNextOrder(context.Background())

		assert.ErrorIs(t, err, ErrQueueIsEmpty)
	})
}

func TestWorkersRepo_UpdateOrderStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetrics := NewMockWorkersMetricsRepoIFace(ctrl)
	mockPool := NewMockPgxPoolIface(ctrl)
	mockPg := NewMockPgInstance(ctrl)
	mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, fn func(context.Context, pgc.PgxPoolIface) error) error {
			return fn(ctx, mockPool)
		}).AnyTimes()

	fixedTime := time.Now()
	repo := &WorkersRepo{
		pg:      mockPg,
		metrics: mockMetrics,
		now:     func() time.Time { return fixedTime },
	}

	t.Run("success_update_processed", func(t *testing.T) {
		orderNum := domain.OrderNumber("123")
		status := domain.OrderStatus("PROCESSED")
		accrual := domain.Amount(100)

		// Мокаем результат Exec (1 строка затронута)
		tag := pgconn.NewCommandTag("UPDATE 1")
		mockPool.EXPECT().Exec(gomock.Any(), sqlUpdateOrderAndBalance, orderNum, status, accrual, fixedTime).Return(tag, nil)

		mockMetrics.EXPECT().ObserveDB(metrics.OpExec, gomock.Any())
		mockMetrics.EXPECT().IncOrderFinalized(status)

		err := repo.UpdateOrderStatus(context.Background(), orderNum, status, accrual)
		assert.NoError(t, err)
	})

	t.Run("already_processed_rows_0", func(t *testing.T) {
		orderNum := domain.OrderNumber("123")
		tag := pgconn.NewCommandTag("UPDATE 0")
		mockPool.EXPECT().Exec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(tag, nil)

		mockMetrics.EXPECT().ObserveDB(metrics.OpExec, gomock.Any())
		// Finalized НЕ должен вызываться, так как RowsAffected == 0 (наша правка в ревью)

		err := repo.UpdateOrderStatus(context.Background(), orderNum, "PROCESSED", 100)
		assert.NoError(t, err)
	})
}
