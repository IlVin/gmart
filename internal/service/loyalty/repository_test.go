package loyalty

import (
	"context"
	"errors"
	"testing"
	"time"

	"gmart/internal/adapters/pgc"
	"gmart/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestLoyaltyRepo_GetBalance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockPool := NewMockPgxPoolIface(ctrl)
	mockRow := NewMockRow(ctrl)

	repo := NewLoyaltyRepo(mockPg, nil)
	userID := domain.UserID(1)

	t.Run("success_get_balance", func(t *testing.T) {
		mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
			return cb(ctx, mockPool)
		})

		mockPool.EXPECT().QueryRow(gomock.Any(), sqlGetBalance, userID).Return(mockRow)
		mockRow.EXPECT().Scan(gomock.Any(), gomock.Any()).DoAndReturn(func(dest ...any) error {
			*(dest[0].(*domain.Amount)) = 100050
			*(dest[1].(*domain.Amount)) = 50025
			return nil
		})

		curr, with, err := repo.GetBalance(context.Background(), userID)
		assert.NoError(t, err)
		assert.Equal(t, domain.Amount(100050), curr)
		assert.Equal(t, domain.Amount(50025), with)
	})

	t.Run("user_not_found_returns_zeros", func(t *testing.T) {
		mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
			return cb(ctx, mockPool)
		})

		mockPool.EXPECT().QueryRow(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockRow)
		mockRow.EXPECT().Scan(gomock.Any(), gomock.Any()).Return(pgx.ErrNoRows)

		curr, with, err := repo.GetBalance(context.Background(), userID)
		assert.NoError(t, err)
		assert.Equal(t, domain.Amount(0), curr)
		assert.Equal(t, domain.Amount(0), with)
	})
}

func TestLoyaltyRepo_Withdraw(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockPool := NewMockPgxPoolIface(ctrl)
	mockRow := NewMockRow(ctrl)

	fixedNow := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	repo := &LoyaltyRepo{
		pg:  mockPg,
		now: func() time.Time { return fixedNow },
	}

	userID := domain.UserID(1)
	order := domain.OrderNumber("12345")
	amount := domain.Amount(100)

	cases := []struct {
		name    string
		dbRes   string
		wantErr error
	}{
		{"success", "success", nil},
		{"insufficient_funds", "no_money", ErrInsufficientFunds},
		{"already_exists", "already_exists", ErrWithdrawConflict},
		{"unexpected", "error_xxx", errors.New("unexpected withdraw status: error_xxx")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
				return cb(ctx, mockPool)
			})

			mockPool.EXPECT().QueryRow(gomock.Any(), sqlWithdraw, userID, amount, order, fixedNow).Return(mockRow)
			mockRow.EXPECT().Scan(gomock.Any()).DoAndReturn(func(dest ...any) error {
				*(dest[0].(*string)) = tc.dbRes
				return nil
			})

			err := repo.Withdraw(context.Background(), userID, order, amount)
			if tc.wantErr != nil {
				assert.Contains(t, err.Error(), tc.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestLoyaltyRepo_GetWithdrawals(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockPool := NewMockPgxPoolIface(ctrl)
	mockRows := NewMockRows(ctrl)

	repo := NewLoyaltyRepo(mockPg, nil)
	userID := domain.UserID(1)

	t.Run("success_history", func(t *testing.T) {
		mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
			return cb(ctx, mockPool)
		})

		mockPool.EXPECT().Query(gomock.Any(), sqlGetWithdrawals, userID).Return(mockRows, nil)

		// Эмулируем 2 строки в ответе
		gomock.InOrder(
			mockRows.EXPECT().Next().Return(true),
			mockRows.EXPECT().Scan(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(dest ...any) error {
				*(dest[0].(*domain.OrderNumber)) = "order1"
				return nil
			}),
			mockRows.EXPECT().Next().Return(true),
			mockRows.EXPECT().Scan(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(dest ...any) error {
				*(dest[0].(*domain.OrderNumber)) = "order2"
				return nil
			}),
			mockRows.EXPECT().Next().Return(false),
		)
		mockRows.EXPECT().Close()
		mockRows.EXPECT().Err().Return(nil)

		res := []domain.Withdrawal{}
		for rs, err := range repo.GetWithdrawals(context.Background(), userID) {
			assert.NoError(t, err)
			res = append(res, rs)
		}
		assert.Len(t, res, 2)
		assert.Equal(t, domain.OrderNumber("order1"), res[0].OrderNumber)
	})
}
