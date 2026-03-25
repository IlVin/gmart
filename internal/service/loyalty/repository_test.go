package loyalty

import (
	"context"
	"testing"
	"time"

	"gmart/internal/domain"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestLoyaltyRepo_GetBalance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockRow := NewMockRow(ctrl)

	repo := NewLoyaltyRepo(mockPg, nil)
	userID := domain.UserID(1)

	t.Run("success_get_balance", func(t *testing.T) {
		// 1. Настраиваем ожидание Fetch у MockPgInstance (вместо PgPool)
		// ps.One внутри вызывает ps.pg.Fetch(ctx, ps.sql, args...)
		mockPg.EXPECT().
			Fetch(gomock.Any(), gomock.Any(), userID).
			Return(mockRow, nil) // возвращаем MockRow

		// 2. Настраиваем Scan у MockRow
		mockRow.EXPECT().
			Scan(gomock.Any(), gomock.Any()).
			DoAndReturn(func(dest ...any) error {
				// Симулируем данные из БД: баланс 500.5, списано 10.0
				*dest[0].(*domain.Amount) = 50050
				*dest[1].(*domain.Amount) = 1000
				return nil
			})

		// Вызываем метод репозитория (который теперь юзает PreparedStatement)
		b, err := repo.GetBalance(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, domain.Amount(50050), b.Current)
		assert.Equal(t, domain.Amount(1000), b.Withdrawn)
	})

	t.Run("user_not_found_returns_zeros", func(t *testing.T) {
		mockPg.EXPECT().
			Fetch(gomock.Any(), gomock.Any(), userID).
			Return(mockRow, nil)

		mockRow.EXPECT().
			Scan(gomock.Any(), gomock.Any()).
			Return(pgx.ErrNoRows)

		b, err := repo.GetBalance(context.Background(), userID)

		assert.NoError(t, err)
		assert.Equal(t, domain.Amount(0), b.Current)
		assert.Equal(t, domain.Amount(0), b.Withdrawn)
	})
}

func TestLoyaltyRepo_Withdraw(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockRow := NewMockRow(ctrl) // Мок для pgx.Row
	mockMetrics := NewMockLoyaltyMetrics(ctrl)

	repo := NewLoyaltyRepo(mockPg, mockMetrics)
	ctx := context.Background()
	userID := domain.UserID(1)
	order := domain.OrderNumber("12345")
	amount := domain.Amount(100)

	t.Run("success", func(t *testing.T) {
		// 1. Ожидаем вызов Fetch (так как ps.One вызывает его)
		// Проверь порядок аргументов в sqlWithdraw: user_id, amount, order_number, now
		mockPg.EXPECT().
			Fetch(ctx, sqlWithdraw, userID, amount, order, gomock.Any()).
			Return(mockRow, nil)

		// 2. Ожидаем Scan, который запишет "success" в переменную status
		mockRow.EXPECT().
			Scan(gomock.Any()). // Один аргумент, так как binder для string возвращает []any{&w}
			DoAndReturn(func(dest ...any) error {
				// Записываем статус в указатель, который передал PreparedStatement
				*dest[0].(*string) = "success"
				return nil
			})

		// 3. Ожидаем вызовы метрик
		mockMetrics.EXPECT().ObserveDB(domain.OpQuery, gomock.Any())
		mockMetrics.EXPECT().IncWithdrawal("success")
		mockMetrics.EXPECT().ObserveWithdrawalAmount(amount)

		err := repo.Withdraw(ctx, userID, order, amount)
		assert.NoError(t, err)
	})

	t.Run("insufficient_funds", func(t *testing.T) {
		mockPg.EXPECT().
			Fetch(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(mockRow, nil)

		mockRow.EXPECT().
			Scan(gomock.Any()).
			DoAndReturn(func(dest ...any) error {
				*dest[0].(*string) = "no_money"
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
	mockRows := NewMockRows(ctrl)

	repo := NewLoyaltyRepo(mockPg, nil)
	userID := domain.UserID(1)

	t.Run("success_history", func(t *testing.T) {
		mockPg.EXPECT().
			Query(gomock.Any(), gomock.Any(), userID). // ctx, sql, $1
			Return(mockRows, nil)

		gomock.InOrder(
			mockRows.EXPECT().Next().Return(true),
			mockRows.EXPECT().Scan(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(dest ...any) error {
					// Заполняем: OrderNumber, Amount, ProcessedAt
					*dest[0].(*domain.OrderNumber) = "12345"
					*dest[1].(*domain.Amount) = domain.NewAmountFromRubles(500)
					*dest[2].(*time.Time) = time.Now()
					return nil
				}),
			mockRows.EXPECT().Next().Return(true),
			mockRows.EXPECT().Scan(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(dest ...any) error {
					*dest[0].(*domain.OrderNumber) = "67890"
					*dest[1].(*domain.Amount) = domain.NewAmountFromRubles(100)
					*dest[2].(*time.Time) = time.Now()
					return nil
				}),
			mockRows.EXPECT().Next().Return(false),
			mockRows.EXPECT().Err().Return(nil),
			mockRows.EXPECT().Close(),
		)

		it := repo.GetWithdrawals(context.Background(), userID)

		count := 0
		for w, err := range it {
			assert.NoError(t, err)
			assert.NotEmpty(t, w.OrderNumber) // Теперь это имеет смысл проверять
			count++
		}
		assert.Equal(t, 2, count)
	})
}
