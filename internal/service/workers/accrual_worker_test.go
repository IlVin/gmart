package workers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gmart/internal/domain"
	"gmart/internal/dto"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestAccrualWrk_DoWork(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockWorkerRepoIFace(ctrl)
	mockMetrics := NewMockWorkerMetricsIFace(ctrl)

	orderNum := domain.OrderNumber("12345")

	t.Run("success_processed", func(t *testing.T) {
		// 1. Настраиваем фейковый сервер Accrual
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Contains(t, r.URL.Path, orderNum.String())
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(dto.AccrualResponse{
				Order:   orderNum,
				Status:  "PROCESSED",
				Accrual: 500,
			})
		}))
		defer ts.Close()

		u, _ := url.Parse(ts.URL)
		wrk := NewAccrualWrk(mockRepo, mockMetrics, u)

		// 2. Ожидания
		mockRepo.EXPECT().AcquireNextOrder(gomock.Any()).Return(orderNum, domain.OrderStatus("NEW"), nil)
		mockRepo.EXPECT().UpdateOrderStatus(gomock.Any(), orderNum, domain.OrderStatus("PROCESSED"), domain.Amount(500)).Return(nil)

		mockMetrics.EXPECT().ObserveRequest("200", gomock.Any())
		mockMetrics.EXPECT().IncProcessed("success")

		// 3. Выполнение
		err := wrk.doWork(context.Background())
		assert.NoError(t, err)
	})

	t.Run("rate_limit_429", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer ts.Close()

		u, _ := url.Parse(ts.URL)
		wrk := NewAccrualWrk(mockRepo, mockMetrics, u)

		mockRepo.EXPECT().AcquireNextOrder(gomock.Any()).Return(orderNum, domain.OrderStatus("NEW"), nil)

		mockMetrics.EXPECT().ObserveRequest("429", gomock.Any())
		mockMetrics.EXPECT().IncRateLimit()
		mockMetrics.EXPECT().IncProcessed("rate_limit")

		start := time.Now().UnixMilli()
		err := wrk.doWork(context.Background())

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rate limited")

		// Проверяем, что sleepUntilMs установился корректно (+2 сек)
		expectedSleep := start + 2000
		assert.GreaterOrEqual(t, wrk.sleepUntilMs.Load(), expectedSleep)
	})

	t.Run("no_content_204", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		defer ts.Close()

		u, _ := url.Parse(ts.URL)
		wrk := NewAccrualWrk(mockRepo, mockMetrics, u)

		mockRepo.EXPECT().AcquireNextOrder(gomock.Any()).Return(orderNum, domain.OrderStatus("NEW"), nil)
		mockMetrics.EXPECT().ObserveRequest("204", gomock.Any())
		mockMetrics.EXPECT().IncProcessed("no_content")

		err := wrk.doWork(context.Background())
		assert.NoError(t, err)
	})
}

func TestAccrualWrk_Lifecycle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := NewMockWorkerRepoIFace(ctrl)
	mockMetrics := NewMockWorkerMetricsIFace(ctrl)
	u, _ := url.Parse("http://localhost:8080")

	wrk := NewAccrualWrk(mockRepo, mockMetrics, u)

	t.Run("shutdown_stops_workers", func(t *testing.T) {
		// Настраиваем бесконечную "пустую очередь"
		mockRepo.EXPECT().AcquireNextOrder(gomock.Any()).Return(domain.OrderNumber(""), domain.OrderStatus(""), ErrQueueIsEmpty).AnyTimes()

		ctx, cancel := context.WithCancel(context.Background())

		// Запускаем 2 воркера
		go wrk.Run(ctx, 2)

		time.Sleep(100 * time.Millisecond)

		// Останавливаем
		cancel()
		wrk.Shutdown()

		assert.True(t, wrk.isShutdown.Load())
	})
}
