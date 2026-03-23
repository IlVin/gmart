package accrual

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"gmart/internal/domain"
	"gmart/internal/dto"

	"github.com/stretchr/testify/assert"
)

func TestClient_Fetch(t *testing.T) {
	tests := []struct {
		name           string
		orderID        string
		serverResponse string
		status         int
		retryAfter     string
		wantResponse   *dto.AccrualResponse
		wantErr        error
		checkErrText   string // Добавим для проверки динамических ошибок
	}{
		{
			name:           "Успешный запрос (200 OK)",
			orderID:        "12345",
			status:         http.StatusOK,
			serverResponse: `{"order": "12345", "status": "PROCESSED", "accrual": 500.5}`,
			wantResponse: &dto.AccrualResponse{
				Order:   "12345",
				Status:  "PROCESSED",
				Accrual: 50050,
			},
			wantErr: nil,
		},
		{
			name:    "Заказ не найден (204 No Content)",
			orderID: "12345",
			status:  http.StatusNoContent,
			wantErr: ErrNoContent,
		},
		{
			name:       "Превышение лимита (429 Too Many Requests)",
			orderID:    "12345",
			status:     http.StatusTooManyRequests,
			retryAfter: "10",
			wantResponse: &dto.AccrualResponse{
				RetryAfter: 10 * time.Second,
			},
			wantErr: ErrTooManyRequests,
		},
		{
			name:    "Ошибка сервера (500 Internal Error)",
			orderID: "12345",
			status:  http.StatusInternalServerError,
			wantErr: ErrInternalError,
		},
		{
			name:         "Неожиданный статус код (403)",
			orderID:      "12345",
			status:       http.StatusForbidden,
			checkErrText: "unexpected status code: 403",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				w.WriteHeader(tt.status)
				if tt.serverResponse != "" {
					_, _ = w.Write([]byte(tt.serverResponse))
				}
			}))
			defer server.Close()

			baseURL, _ := url.Parse(server.URL)
			client := NewClient(baseURL)

			res, err := client.Fetch(context.Background(), domain.OrderNumber(tt.orderID))

			// Проверка ошибок
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
			} else if tt.checkErrText != "" {
				assert.ErrorContains(t, err, tt.checkErrText)
			} else {
				assert.NoError(t, err)
			}

			// Проверка данных и задержки
			assert.Equal(t, tt.wantResponse, res)
		})
	}
}
