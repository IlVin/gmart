package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrderStatus_String(t *testing.T) {
	// Определим константы для теста, если они есть в пакете,
	// либо проверим прямое приведение типов.
	tests := []struct {
		name   string
		status OrderStatus
		want   string
	}{
		{
			name:   "new status",
			status: OrderStatus("NEW"),
			want:   "NEW",
		},
		{
			name:   "processing status",
			status: OrderStatus("PROCESSING"),
			want:   "PROCESSING",
		},
		{
			name:   "invalid status",
			status: OrderStatus("INVALID"),
			want:   "INVALID",
		},
		{
			name:   "processed status",
			status: OrderStatus("PROCESSED"),
			want:   "PROCESSED",
		},
		{
			name:   "empty status",
			status: OrderStatus(""),
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Проверка метода String()
			assert.Equal(t, tt.want, tt.status.String())

			// Проверка явного приведения к string (используется в SQL запросах)
			assert.Equal(t, tt.want, string(tt.status))
		})
	}
}
