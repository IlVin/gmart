package loyalty

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"gmart/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestLoyalty_GetBalance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockLoyaltyRepoIface(ctrl)
	// Создаем объект напрямую, так как NewLoyalty создает реальный репозиторий
	svc := &Loyalty{repo: mockRepo}

	userID := domain.UserID(1)

	t.Run("Success", func(t *testing.T) {
		mockRepo.EXPECT().
			GetBalance(gomock.Any(), userID).
			Return(domain.Balance{Current: domain.Amount(10050), Withdrawn: domain.Amount(5000)}, nil).
			Times(1)

		b, err := svc.GetBalance(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, domain.Amount(10050), b.Current)
		assert.Equal(t, domain.Amount(5000), b.Withdrawn)
	})
}

func TestLoyalty_Withdraw(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockLoyaltyRepoIface(ctrl)
	svc := &Loyalty{repo: mockRepo}

	userID := domain.UserID(1)
	validOrder := domain.OrderNumber("2377225624") // Валидный по Луну
	invalidOrder := domain.OrderNumber("12345")    // Невалидный
	amount := domain.Amount(50000)

	t.Run("Success", func(t *testing.T) {
		mockRepo.EXPECT().
			Withdraw(gomock.Any(), userID, validOrder, amount).
			Return(nil).
			Times(1)

		err := svc.Withdraw(context.Background(), userID, validOrder, amount)
		assert.NoError(t, err)
	})

	t.Run("Invalid Order Number (Luhn)", func(t *testing.T) {
		// Репозиторий НЕ должен вызываться
		err := svc.Withdraw(context.Background(), userID, invalidOrder, amount)
		assert.ErrorIs(t, err, ErrInvalidOrderNumber)
	})

	t.Run("Insufficient Funds from Repo", func(t *testing.T) {
		// Имитируем ошибку баланса из БД
		errFunds := errors.New("insufficient balance")
		mockRepo.EXPECT().
			Withdraw(gomock.Any(), userID, validOrder, amount).
			Return(errFunds).
			Times(1)

		err := svc.Withdraw(context.Background(), userID, validOrder, amount)
		assert.Error(t, err)
		assert.Equal(t, errFunds, err)
	})
}

func TestLoyalty_GetWithdrawals(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockLoyaltyRepoIface(ctrl)
	svc := &Loyalty{repo: mockRepo}

	userID := domain.UserID(1)

	t.Run("Success", func(t *testing.T) {
		expected := []domain.Withdrawal{
			{
				OrderNumber: "2377225624",
				Amount:      50000,
				ProcessedAt: time.Now(),
			},
		}

		// 1. Создаем итератор из слайса (Go 1.23 style)
		// Т.к. Seq2 ожидает два значения (V, error),
		// нам нужно подготовить функцию, которая будет их отдавать.
		iterator := func(yield func(domain.Withdrawal, error) bool) {
			for _, w := range expected {
				if !yield(w, nil) {
					return
				}
			}
		}

		// 2. Исправляем Return: теперь возвращаем ТОЛЬКО итератор (1 аргумент)
		mockRepo.EXPECT().
			GetWithdrawals(gomock.Any(), userID).
			Return(iter.Seq2[domain.Withdrawal, error](iterator)). // Приводим к типу
			Times(1)

		res := []domain.Withdrawal{}
		// 3. Твой код в цикле уже правильный
		for rs, err := range svc.GetWithdrawals(context.Background(), userID) {
			require.NoError(t, err)
			res = append(res, rs)
		}

		assert.Len(t, res, 1)
		assert.Equal(t, domain.OrderNumber("2377225624"), res[0].OrderNumber)
	})
}
