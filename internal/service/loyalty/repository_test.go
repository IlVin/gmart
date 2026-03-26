package loyalty

import (
	"context"
	"testing"
	"time"

	pgc "gmart/internal/adapters/pgc"
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
	repo := NewLoyaltyRepo(mockPg, nil)

	ctx := context.Background()
	userID := domain.UserID(100)

	t.Run("User_Not_Found_Returns_Zero_Balance", func(t *testing.T) {
		mockPg.EXPECT().PgPool(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
			return cb(ctx, mockPool)
		})

		mockRow := NewMockRow(ctrl)
		mockPool.EXPECT().QueryRow(ctx, sqlGetBalance, userID).Return(mockRow)
		mockRow.EXPECT().Scan(gomock.Any()).Return(pgx.ErrNoRows)

		balance, err := repo.GetBalance(ctx, userID)
		assert.NoError(t, err)
		assert.Equal(t, domain.NewAmountFromCoins(0), balance.Current)
	})
}

func TestLoyaltyRepo_Withdraw(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockPool := NewMockPgxPoolIface(ctrl)
	mockMetrics := NewMockLoyaltyMetrics(ctrl)

	fixedTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	repo := NewLoyaltyRepo(mockPg, mockMetrics)
	repo.now = func() time.Time { return fixedTime }

	ctx := context.Background()
	userID := domain.UserID(100)
	order := domain.OrderNumber("12345")
	amount := domain.Amount(500)

	t.Run("Success_Withdrawal", func(t *testing.T) {
		// 1. Мокаем PgPool, который прокидывает наш mockPool в коллбек
		mockPg.EXPECT().
			PgPool(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
				return cb(ctx, mockPool)
			})

		// 2. Мокаем выполнение запроса внутри пула
		mockRow := NewMockRow(ctrl)
		mockPool.EXPECT().
			QueryRow(ctx, sqlWithdraw, userID, amount, order, fixedTime).
			Return(mockRow)

		// 3. Имитируем возврат статуса "success" через Scan
		mockRow.EXPECT().Scan(gomock.Any()).DoAndReturn(func(dest ...any) error {
			*(dest[0].(*string)) = "success"
			return nil
		})

		// Ожидаем метрики
		mockMetrics.EXPECT().ObserveDB(domain.OpQuery, gomock.Any())
		mockMetrics.EXPECT().IncWithdrawal("success")
		mockMetrics.EXPECT().ObserveWithdrawalAmount(amount)

		err := repo.Withdraw(ctx, userID, order, amount)
		assert.NoError(t, err)
	})

	t.Run("Insufficient_Funds", func(t *testing.T) {
		mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
			return cb(ctx, mockPool)
		})

		mockRow := NewMockRow(ctrl)
		mockPool.EXPECT().QueryRow(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockRow)

		mockRow.EXPECT().Scan(gomock.Any()).DoAndReturn(func(dest ...any) error {
			*(dest[0].(*string)) = "no_money"
			return nil
		})

		mockMetrics.EXPECT().ObserveDB(gomock.Any(), gomock.Any())
		mockMetrics.EXPECT().IncWithdrawal("insufficient_funds")

		err := repo.Withdraw(ctx, userID, order, amount)
		assert.ErrorIs(t, err, ErrInsufficientFunds)
	})
}

func TestLoyaltyRepo_GetWithdrawals(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockPool := NewMockPgxPoolIface(ctrl)
	mockRows := NewMockRows(ctrl)

	repo := NewLoyaltyRepo(mockPg, nil)
	userID := domain.UserID(1)
	ctx := context.Background()

	t.Run("success_history", func(t *testing.T) {
		// 1. Мокаем PgPool, чтобы прокинуть mockPool внутрь pgc.QueryAll
		mockPg.EXPECT().
			PgPool(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
				return cb(ctx, mockPool)
			})

		// 2. Ожидаем вызов Query у пула
		mockPool.EXPECT().
			Query(ctx, sqlGetWithdrawals, userID).
			Return(mockRows, nil)

		// 3. Настраиваем итерацию по строкам
		// Важно: pgc.QueryAll вызывает Scan(aq.binder(&item)...)
		// aq.binder для domain.Withdrawal возвращает []any{&OrderNumber, &Amount, &ProcessedAt}
		gomock.InOrder(
			mockRows.EXPECT().Next().Return(true),
			mockRows.EXPECT().Scan(gomock.Any()). // Binder передает один слайс как variadic dest...
								DoAndReturn(func(dest ...any) error {
					*(dest[0].(*domain.OrderNumber)) = "12345"
					*(dest[1].(*domain.Amount)) = 500
					*(dest[2].(*time.Time)) = time.Now()
					return nil
				}),
			mockRows.EXPECT().Next().Return(true),
			mockRows.EXPECT().Scan(gomock.Any()).
				DoAndReturn(func(dest ...any) error {
					*(dest[0].(*domain.OrderNumber)) = "67890"
					*(dest[1].(*domain.Amount)) = 100
					*(dest[2].(*time.Time)) = time.Now()
					return nil
				}),
			mockRows.EXPECT().Next().Return(false),
		)

		mockRows.EXPECT().Err().Return(nil)
		mockRows.EXPECT().Close()

		it := repo.GetWithdrawals(ctx, userID)

		count := 0
		for w, err := range it {
			assert.NoError(t, err)
			if count == 0 {
				assert.Equal(t, domain.OrderNumber("12345"), w.OrderNumber)
			}
			count++
		}
		assert.Equal(t, 2, count)
	})
}
