package user

import (
	"context"
	"testing"
	"time"

	"gmart/internal/domain"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestUser_Handlers(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockAuthRepoIFace(ctrl)
	mockTokenGen := NewMockTokenGeneratorIFace(ctrl)

	// Инициализируем сервис User с моками
	u := &User{
		authRepo:       mockRepo,
		tokenGenerator: mockTokenGen,
	}

	t.Run("SignUp: Success 200", func(t *testing.T) {
		handler := u.signUpHandler()

		in := &signUpInput{}
		in.Body.Login = "new_user"
		in.Body.Password = "secure_password_123"
		in.UserAgent = "Go-Test"

		userID := domain.UserID(1)
		sessionID := domain.SessionID(uuid.New())
		token := domain.Token("jwt_token")

		// Настраиваем ожидания для цепочки вызовов внутри SignUp
		mockRepo.EXPECT().
			SignUp(gomock.Any(), in.Body.Login, in.Body.Password, in.UserAgent, gomock.Any()).
			Return(userID, sessionID, nil)

		mockTokenGen.EXPECT().
			Generate(userID).
			Return(token, nil)

		mockRepo.EXPECT().ExpDuration().Return(24 * time.Hour)

		resp, err := handler(context.Background(), in)

		assert.NoError(t, err)
		assert.Equal(t, token, resp.Body.Token)
		assert.Contains(t, resp.SetCookie, "session_id=")
		assert.Equal(t, "Bearer jwt_token", resp.Authorization)
	})

	t.Run("SignUp: Conflict 409", func(t *testing.T) {
		handler := u.signUpHandler()

		in := &signUpInput{}
		in.Body.Login = "existing_user"

		mockRepo.EXPECT().
			SignUp(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(domain.UserID(0), domain.SessionID(uuid.Nil), ErrUserAlreadyExists)

		resp, err := handler(context.Background(), in)

		assert.Nil(t, resp)
		var humaErr huma.StatusError
		if assert.ErrorAs(t, err, &humaErr) {
			assert.Equal(t, 409, humaErr.GetStatus())
		}
	})

	t.Run("SignIn: Success 200", func(t *testing.T) {
		handler := u.signInHandler()

		in := &signInInput{}
		in.Body.Login = "active_user"
		in.Body.Password = "correct_pass"

		userID := domain.UserID(2)
		sessionID := domain.SessionID(uuid.New())
		token := domain.Token("new_jwt_token")

		// ИСПРАВЛЕНИЕ: Твой код вызывает SignInSleep, а не SignIn
		mockRepo.EXPECT().
			SignInSleep(gomock.Any(), in.Body.Login, in.Body.Password, gomock.Any(), gomock.Any()).
			Return(userID, sessionID, nil)

		mockTokenGen.EXPECT().Generate(userID).Return(token, nil)
		mockRepo.EXPECT().ExpDuration().Return(24 * time.Hour)

		resp, err := handler(context.Background(), in)

		assert.NoError(t, err)
		assert.Equal(t, token, resp.Body.Token)
		assert.Equal(t, 200, resp.Body.Code)
	})

	t.Run("SignIn: Unauthorized 401", func(t *testing.T) {
		handler := u.signInHandler()

		in := &signInInput{}
		in.Body.Login = "wrong_user"

		// Здесь тоже меняем на SignInSleep
		mockRepo.EXPECT().
			SignInSleep(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(domain.UserID(0), domain.SessionID(uuid.Nil), ErrInvalidPassword)

		resp, err := handler(context.Background(), in)

		assert.Nil(t, resp)
		var humaErr huma.StatusError
		if assert.ErrorAs(t, err, &humaErr) {
			assert.Equal(t, 401, humaErr.GetStatus())
		}
	})
}
