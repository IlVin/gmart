package orders

import (
	"context"
	"testing"
	"time"

	"gmart/internal/adapters/pgc"
	"gmart/internal/domain"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestOrdersRepo_Upload(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockPool := NewMockPgxPoolIface(ctrl)
	mockRow := NewMockRow(ctrl)

	fixedNow := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	repo := &OrdersRepo{
		pg:  mockPg,
		now: func() time.Time { return fixedNow },
	}

	userID := domain.UserID(42)
	orderNum := domain.OrderNumber("12345678903")

	t.Run("success_new_order", func(t *testing.T) {
		mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
			return cb(ctx, mockPool)
		})

		mockPool.EXPECT().QueryRow(gomock.Any(), sqlInsertIntoOrders, orderNum, userID, domain.StatusNew, fixedNow).Return(mockRow)

		// ИСПРАВЛЕНИЕ: DoAndReturn принимает (dest ...any) как один аргумент []any
		mockRow.EXPECT().Scan(gomock.Any(), gomock.Any()).DoAndReturn(func(dest ...any) error {
			// dest[0] это *domain.UserID, dest[1] это *bool
			*(dest[0].(*domain.UserID)) = userID
			*(dest[1].(*bool)) = false
			return nil
		})

		err := repo.Upload(context.Background(), userID, orderNum)
		assert.NoError(t, err)
	})

	t.Run("error_conflict_with_another_user", func(t *testing.T) {
		mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
			return cb(ctx, mockPool)
		})

		mockPool.EXPECT().QueryRow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockRow)

		mockRow.EXPECT().Scan(gomock.Any(), gomock.Any()).DoAndReturn(func(dest ...any) error {
			*(dest[0].(*domain.UserID)) = domain.UserID(99)
			*(dest[1].(*bool)) = true
			return nil
		})

		err := repo.Upload(context.Background(), userID, orderNum)
		assert.ErrorIs(t, err, ErrOrderConflict)
	})
}

func TestOrdersRepo_List(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockPool := NewMockPgxPoolIface(ctrl)
	mockRows := NewMockRows(ctrl)

	repo := NewOrdersRepo(mockPg, nil)
	userID := domain.UserID(1)

	t.Run("success_return_list", func(t *testing.T) {
		mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
			return cb(ctx, mockPool)
		})

		mockPool.EXPECT().Query(gomock.Any(), sqlSelectOrdersByUserID, userID).Return(mockRows, nil)

		fixedTime := time.Now()

		gomock.InOrder(
			mockRows.EXPECT().Next().Return(true),
			// ИСПРАВЛЕНИЕ: Используем dest ...any и индексный доступ
			mockRows.EXPECT().Scan(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(dest ...any) error {
				*(dest[0].(*domain.OrderNumber)) = "123"
				*(dest[1].(*domain.OrderStatus)) = "NEW"
				*(dest[2].(*domain.Amount)) = 0
				*(dest[3].(*time.Time)) = fixedTime
				return nil
			}),
			mockRows.EXPECT().Next().Return(false),
		)

		mockRows.EXPECT().Close().Times(1)
		mockRows.EXPECT().Err().Return(nil).Times(1)

		res, err := repo.List(context.Background(), userID)

		assert.NoError(t, err)
		assert.Len(t, res, 1)
		assert.Equal(t, domain.OrderNumber("123"), res[0].OrderNumber)
		assert.Equal(t, domain.OrderStatus("NEW"), res[0].Status)
	})
}
