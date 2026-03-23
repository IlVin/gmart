package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatus_Constants(t *testing.T) {
	// Тест фиксирует значения констант, чтобы предотвратить их случайное изменение
	// при рефакторинге, так как они являются частью API и схемы БД.

	tests := []struct {
		name     string
		constant string
		want     string
	}{
		{
			name:     "StatusNew value",
			constant: StatusNew,
			want:     "NEW",
		},
		{
			name:     "StatusProcessing value",
			constant: StatusProcessing,
			want:     "PROCESSING",
		},
		{
			name:     "StatusInvalid value",
			constant: StatusInvalid,
			want:     "INVALID",
		},
		{
			name:     "StatusProcessed value",
			constant: StatusProcessed,
			want:     "PROCESSED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.constant)
		})
	}
}

func TestStatus_Integrity(t *testing.T) {
	// Проверяем, что константы можно использовать в типе OrderStatus
	t.Run("constants match OrderStatus type", func(t *testing.T) {
		var status OrderStatus = StatusNew
		assert.Equal(t, "NEW", status.String())

		status = StatusProcessed
		assert.Equal(t, "PROCESSED", status.String())
	})
}
