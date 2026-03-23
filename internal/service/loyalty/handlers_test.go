package loyalty

import (
	"context"
	"testing"

	"gmart/internal/domain"
	"gmart/internal/model/auth"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestLoyalty_Handlers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockLoyaltyRepoIface(ctrl)
	svc := &Loyalty{repo: mockRepo}

	userID := domain.UserID(42)
	authCtx := context.WithValue(context.Background(), auth.UserID, userID)

	t.Run("GetBalance: Unauthorized 401", func(t *testing.T) {
		handler := svc.getBalanceHandler()

		resp, err := handler(context.Background(), &balanceInput{})

		assert.Nil(t, resp)
		// В Huma v2 ошибки реализуют интерфейс со методом GetStatus()
		var humaErr huma.StatusError
		if assert.ErrorAs(t, err, &humaErr) {
			assert.Equal(t, 401, humaErr.GetStatus())
		}
	})

	t.Run("Withdraw: Insufficient Funds 402", func(t *testing.T) {
		handler := svc.withdrawHandler()
		in := &withdrawInput{}
		in.Body.Order = "12345678903"
		in.Body.Sum = 99999

		mockRepo.EXPECT().
			Withdraw(gomock.Any(), userID, in.Body.Order, in.Body.Sum).
			Return(ErrInsufficientFunds)

		resp, err := handler(authCtx, in)

		assert.Nil(t, resp)
		var humaErr huma.StatusError
		if assert.ErrorAs(t, err, &humaErr) {
			assert.Equal(t, 402, humaErr.GetStatus())
		}
	})

	t.Run("Withdraw: Success 200", func(t *testing.T) {
		handler := svc.withdrawHandler()
		orderNum := domain.OrderNumber("12345678903")
		sum := domain.Amount(1000)

		in := &withdrawInput{}
		in.Body.Order = orderNum
		in.Body.Sum = sum

		mockRepo.EXPECT().
			Withdraw(gomock.Any(), userID, orderNum, sum).
			Return(nil)

		resp, err := handler(authCtx, in)

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.Status)
	})
}
