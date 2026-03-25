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
	mockRows := NewMockRows(ctrl)

	repo := NewLoyaltyRepo(mockPg, nil)
	userID := domain.UserID(1)

	t.Run("success_history", func(t *testing.T) {

		mockPg.EXPECT().
			Query(gomock.Any(), gomock.Any(), userID). // sql и аргумент
			Return(mockRows, nil)

		// Эмулируем 2 строки в ответе
		gomock.InOrder(
			mockRows.EXPECT().Next().Return(true),
			mockRows.EXPECT().Scan(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(dest ...any) error {
					// Заполняем поля первой строки
					return nil
				}),
			mockRows.EXPECT().Next().Return(true),
			mockRows.EXPECT().Scan(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(dest ...any) error {
					// Заполняем поля второй строки
					return nil
				}),
			mockRows.EXPECT().Next().Return(false), // Конец данных
			mockRows.EXPECT().Err().Return(nil),    // Финальная проверка ошибок
			mockRows.EXPECT().Close(),              // ОБЯЗАТЕЛЬНО: итератор всегда вызывает Close
		)

		// Вызов метода репозитория
		it := repo.GetWithdrawals(context.Background(), userID)

		// Проверка итерации
		for _, err := range it {
			assert.NoError(t, err)
		}
	})
}
