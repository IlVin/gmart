package workers

import (
	"context"
	"errors"
	"testing"
	"time"

	"gmart/internal/adapters/pgc"
	"gmart/internal/domain"

	pgconn "github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestWorkersRepo_AcquireNextOrder(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockPool := NewMockPgxPoolIface(ctrl)
	mockRow := NewMockRow(ctrl)
	mockMetrics := NewMockWorkersMetricsRepoIFace(ctrl)

	fixedNow := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	repo := &WorkersRepo{
		pg:      mockPg,
		metrics: mockMetrics,
		now:     func() time.Time { return fixedNow },
	}

	t.Run("success_acquire", func(t *testing.T) {
		expectedNumber := domain.OrderNumber("12345")
		expectedStatus := domain.OrderStatus("NEW")

		mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
			return cb(ctx, mockPool)
		})

		mockPool.EXPECT().QueryRow(gomock.Any(), sqlAcquireNextOrder, fixedNow).Return(mockRow)
		mockRow.EXPECT().Scan(gomock.Any(), gomock.Any()).DoAndReturn(func(dest ...any) error {
			*(dest[0].(*domain.OrderNumber)) = expectedNumber
			*(dest[1].(*domain.OrderStatus)) = expectedStatus
			return nil
		})

		// Ожидаем метрики
		mockMetrics.EXPECT().ObserveDB(domain.OpQuery, gomock.Any())
		mockMetrics.EXPECT().IncAcquireAttempt(true)

		num, status, err := repo.AcquireNextOrder(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, expectedNumber, num)
		assert.Equal(t, expectedStatus, status)
	})

	t.Run("db_error_other_than_norows", func(t *testing.T) {
		dbErr := errors.New("connection lost")
		mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
			return cb(ctx, mockPool)
		})

		mockPool.EXPECT().QueryRow(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockRow)
		mockRow.EXPECT().Scan(gomock.Any(), gomock.Any()).Return(dbErr)
		mockMetrics.EXPECT().ObserveDB(domain.OpQuery, gomock.Any())

		_, _, err := repo.AcquireNextOrder(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "acquire order fail")
	})
}

func TestWorkersRepo_UpdateOrderStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockPool := NewMockPgxPoolIface(ctrl)
	mockMetrics := NewMockWorkersMetricsRepoIFace(ctrl)

	fixedNow := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	repo := &WorkersRepo{
		pg:      mockPg,
		metrics: mockMetrics,
		now:     func() time.Time { return fixedNow },
	}

	orderNum := domain.OrderNumber("12345")
	status := domain.OrderStatus("PROCESSED")
	amount := domain.Amount(500)

	t.Run("success_update", func(t *testing.T) {
		mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
			return cb(ctx, mockPool)
		})

		// Возвращаем тег с 1 затронутой строкой
		tag := pgconn.NewCommandTag("UPDATE 1")
		mockPool.EXPECT().Exec(gomock.Any(), sqlUpdateOrderAndBalance, orderNum, status, amount, fixedNow).Return(tag, nil)

		// Ожидаем метрики
		mockMetrics.EXPECT().ObserveDB(domain.OpExec, gomock.Any())
		mockMetrics.EXPECT().IncOrderFinalized(status)

		err := repo.UpdateOrderStatus(context.Background(), orderNum, status, amount)
		assert.NoError(t, err)
	})

	t.Run("order_already_processed_no_rows_affected", func(t *testing.T) {
		mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
			return cb(ctx, mockPool)
		})

		tag := pgconn.NewCommandTag("UPDATE 0")
		mockPool.EXPECT().Exec(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(tag, nil)
		mockMetrics.EXPECT().ObserveDB(domain.OpExec, gomock.Any())

		err := repo.UpdateOrderStatus(context.Background(), orderNum, status, amount)
		assert.NoError(t, err) // Репозиторий просто логгирует это, но не возвращает ошибку
	})
}
