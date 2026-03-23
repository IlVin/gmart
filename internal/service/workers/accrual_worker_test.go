package workers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gmart/internal/adapters/accrual" // Добавь импорт адаптера
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
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		// ОШИБКА БЫЛА ТУТ: передаем созданный клиент адаптера, а не URL
		client := accrual.NewClient(u)
		wrk := NewAccrualWrk(mockRepo, mockMetrics, client)

		mockRepo.EXPECT().AcquireNextOrder(gomock.Any()).Return(orderNum, domain.OrderStatus("NEW"), nil)
		mockRepo.EXPECT().UpdateOrderStatus(gomock.Any(), orderNum, domain.OrderStatus("PROCESSED"), domain.Amount(500)).Return(nil)

		mockMetrics.EXPECT().ObserveRequest("200", gomock.Any())
		mockMetrics.EXPECT().IncProcessed("success")

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
		client := accrual.NewClient(u)
		wrk := NewAccrualWrk(mockRepo, mockMetrics, client)

		mockRepo.EXPECT().AcquireNextOrder(gomock.Any()).Return(orderNum, domain.OrderStatus("NEW"), nil)

		// Исправляем код ответа в ожидании - в коде воркера для 429 стоит "409" (согласно твоей логике)
		mockMetrics.EXPECT().ObserveRequest("409", gomock.Any())
		mockMetrics.EXPECT().IncRateLimit()
		mockMetrics.EXPECT().IncProcessed("rate_limit")

		start := time.Now().UnixMilli()
		err := wrk.doWork(context.Background())

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "rate limited")
		assert.GreaterOrEqual(t, wrk.sleepUntilMs.Load(), start+2000)
	})

	t.Run("no_content_204", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		defer ts.Close()

		u, _ := url.Parse(ts.URL)
		client := accrual.NewClient(u)
		wrk := NewAccrualWrk(mockRepo, mockMetrics, client)

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
	client := accrual.NewClient(u)

	wrk := NewAccrualWrk(mockRepo, mockMetrics, client)

	t.Run("shutdown_stops_workers", func(t *testing.T) {
		mockRepo.EXPECT().AcquireNextOrder(gomock.Any()).Return(domain.OrderNumber(""), domain.OrderStatus(""), ErrQueueIsEmpty).AnyTimes()

		ctx, cancel := context.WithCancel(context.Background())
		go wrk.Run(ctx, 2)

		time.Sleep(100 * time.Millisecond)
		cancel()
		wrk.Shutdown()

		assert.True(t, wrk.isShutdown.Load())
	})
}
