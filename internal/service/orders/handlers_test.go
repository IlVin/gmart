package orders

import (
	"context"
	"testing"

	"gmart/internal/domain"
	"gmart/internal/model/auth"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestOrders_Handlers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Используем точное имя из твоего сгенерированного мока
	mockRepo := NewMockOrdersRepoIFace(ctrl)
	svc := &Orders{ordersRepo: mockRepo}

	userID := domain.UserID(100)
	authCtx := context.WithValue(context.Background(), auth.UserID, userID)

	t.Run("Upload: Success 202", func(t *testing.T) {
		handler := svc.ordersUploadHandler()
		// Номер, проходящий проверку Луна
		validOrder := "12345678903"

		in := &ordersUploadInput{
			RawBody: []byte(validOrder),
		}

		mockRepo.EXPECT().
			Upload(gomock.Any(), userID, domain.OrderNumber(validOrder)).
			Return(nil)

		resp, err := handler(authCtx, in)

		assert.NoError(t, err)
		assert.Equal(t, 202, resp.Status)
		assert.Equal(t, "Accepted", resp.Body.Status)
	})

	t.Run("Upload: Already Uploaded 200", func(t *testing.T) {
		handler := svc.ordersUploadHandler()
		validOrder := "12345678903"
		in := &ordersUploadInput{RawBody: []byte(validOrder)}

		mockRepo.EXPECT().
			Upload(gomock.Any(), userID, domain.OrderNumber(validOrder)).
			Return(ErrOrderAlreadyUploaded)

		resp, err := handler(authCtx, in)

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.Status)
		assert.Equal(t, "AlreadyUploaded", resp.Body.Status)
	})

	t.Run("Upload: Conflict 409", func(t *testing.T) {
		handler := svc.ordersUploadHandler()
		validOrder := "12345678903"
		in := &ordersUploadInput{RawBody: []byte(validOrder)}

		mockRepo.EXPECT().
			Upload(gomock.Any(), userID, domain.OrderNumber(validOrder)).
			Return(ErrOrderConflict)

		resp, err := handler(authCtx, in)

		assert.Nil(t, resp)
		var humaErr huma.StatusError
		if assert.ErrorAs(t, err, &humaErr) {
			assert.Equal(t, 409, humaErr.GetStatus())
		}
	})

	t.Run("Upload: Invalid Luhn 422", func(t *testing.T) {
		handler := svc.ordersUploadHandler()
		invalidOrder := "12345" // Не проходит Луна
		in := &ordersUploadInput{RawBody: []byte(invalidOrder)}

		// Репозиторий не должен вызываться, так как luhn.IsValid вернет false
		resp, err := handler(authCtx, in)

		assert.Nil(t, resp)
		var humaErr huma.StatusError
		if assert.ErrorAs(t, err, &humaErr) {
			assert.Equal(t, 422, humaErr.GetStatus())
		}
	})

	t.Run("List: Success 200", func(t *testing.T) {
		handler := svc.ordersListHandler()
		mockList := []domain.Order{
			{OrderNumber: "123", Status: "NEW"},
		}

		mockRepo.EXPECT().
			List(gomock.Any(), userID).
			Return(mockList, nil)

		resp, err := handler(authCtx, &ordersListInput{})

		assert.NoError(t, err)
		assert.Equal(t, 200, resp.Status)
		assert.Len(t, resp.Body, 1)
	})

	t.Run("List: Empty 204", func(t *testing.T) {
		handler := svc.ordersListHandler()

		mockRepo.EXPECT().
			List(gomock.Any(), userID).
			Return(nil, ErrOrderListIsEmpty)

		resp, err := handler(authCtx, &ordersListInput{})

		assert.NoError(t, err)
		assert.Equal(t, 204, resp.Status)
		assert.Empty(t, resp.Body)
	})
}
