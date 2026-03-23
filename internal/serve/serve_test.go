package serve

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gmart/internal/dto"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestBootstrap(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	// Репозитории вызывают PgPool при инициализации в конструкторах
	mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	input := &Input{
		Options: &dto.CLIOptions{
			AccrualSystemAddress:  "http://localhost:8080",
			JwtSecretKey:          "secret-key-must-be-32-chars-long-!!",
			JwtTTL:                time.Hour,
			HttpReadHeaderTimeout: time.Second,
			HttpIdleTimeout:       time.Second,
		},
		Pg:         mockPg,
		MetricsReg: prometheus.NewRegistry(),
	}

	t.Run("successful bootstrap", func(t *testing.T) {
		handler, worker, err := bootstrap(context.Background(), input)
		require.NoError(t, err)
		assert.NotNil(t, handler)
		assert.NotNil(t, worker)

		// Проверяем, что эндпоинт метрик зарегистрирован
		req := httptest.NewRequest("GET", "/metrics", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("invalid accrual url", func(t *testing.T) {
		badInput := *input
		badInput.Options = &dto.CLIOptions{
			AccrualSystemAddress: " : invalid",
		}
		_, _, err := bootstrap(context.Background(), &badInput)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cannot parse Accrual System Address")
	})

	t.Run("invalid jwt secret", func(t *testing.T) {
		badInput := *input
		badInput.Options = &dto.CLIOptions{
			AccrualSystemAddress: "http://localhost",
			JwtSecretKey:         "short", // Меньше 32 байт вызовет ошибку в твоем auth.NewTokenGenerator
			JwtTTL:               time.Hour,
		}
		_, _, err := bootstrap(context.Background(), &badInput)
		assert.Error(t, err)
	})
}

func TestServe_Lifecycle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockPg.EXPECT().PgPool(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockPg.EXPECT().Close().Return(nil).AnyTimes()

	input := &Input{
		Options: &dto.CLIOptions{
			RunAddress:            "127.0.0.1:0", // Случайный порт
			AccrualSystemAddress:  "http://localhost:8080",
			JwtSecretKey:          "secret-key-must-be-32-chars-long-!!",
			JwtTTL:                time.Hour,
			AccrualWorkers:        0, // Отключаем воркеры для чистоты теста
			HttpReadHeaderTimeout: time.Second,
			HttpIdleTimeout:       time.Second,
		},
		Pg:         mockPg,
		MetricsReg: prometheus.NewRegistry(),
	}

	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error, 1)
	go func() {
		errChan <- Serve(ctx, input)
	}()

	// Даем серверу время на запуск
	time.Sleep(100 * time.Millisecond)

	// Инициируем Graceful Shutdown
	cancel()

	select {
	case err := <-errChan:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Server shutdown took too long")
	}
}

func TestInitHuma(t *testing.T) {
	mux := http.NewServeMux()
	api := InitHuma(mux)
	require.NotNil(t, api)

	assert.Equal(t, "GopherMart API", api.OpenAPI().Info.Title)
	assert.Contains(t, api.OpenAPI().Components.SecuritySchemes, "bearer")
}
