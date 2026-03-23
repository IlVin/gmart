package workers

import (
	"context"
	"testing"
	"time"

	"gmart/internal/adapters/pgc"
	"gmart/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestWorkersRepo_AcquireNextOrder(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockPool := NewMockPgxPoolIface(ctrl)
	mockRow := NewMockRow(ctrl)
	// Если метрики не важны для логики, можно использовать nil или замокать их

	fixedNow := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	repo := &WorkersRepo{
		pg:  mockPg,
		now: func() time.Time { return fixedNow },
	}

	t.Run("success_acquire", func(t *testing.T) {
		expectedNumber := domain.OrderNumber("12345")
		expectedStatus := domain.OrderStatus("NEW")

		// 1. Ожидаем вызов PgPool и выполняем коллбек, передавая наш mockPool
		mockPg.EXPECT().
			PgPool(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
				return cb(ctx, mockPool)
			})

		// 2. Ожидаем QueryRow внутри коллбека
		mockPool.EXPECT().
			QueryRow(gomock.Any(), sqlAcquireNextOrder, fixedNow).
			Return(mockRow)

		// 3. Ожидаем Scan и записываем значения в аргументы (указатели)
		mockRow.EXPECT().
			Scan(gomock.Any(), gomock.Any()).
			DoAndReturn(func(dest ...any) error {
				*(dest[0].(*domain.OrderNumber)) = expectedNumber
				*(dest[1].(*domain.OrderStatus)) = expectedStatus
				return nil
			})

		num, status, err := repo.AcquireNextOrder(context.Background())

		assert.NoError(t, err)
		assert.Equal(t, expectedNumber, num)
		assert.Equal(t, expectedStatus, status)
	})

	t.Run("queue_empty", func(t *testing.T) {
		mockPg.EXPECT().
			PgPool(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
				return cb(ctx, mockPool)
			})

		mockPool.EXPECT().
			QueryRow(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(mockRow)

		// Возвращаем стандартную ошибку pgx, что строк нет
		mockRow.EXPECT().Scan(gomock.Any(), gomock.Any()).Return(pgx.ErrNoRows)

		_, _, err := repo.AcquireNextOrder(context.Background())
		assert.ErrorIs(t, err, ErrQueueIsEmpty)
	})
}
