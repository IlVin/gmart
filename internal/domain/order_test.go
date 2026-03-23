package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOrder_JSON(t *testing.T) {
	fixedTime := time.Date(2024, 3, 23, 12, 0, 0, 0, time.UTC)

	t.Run("full order serialization", func(t *testing.T) {
		order := Order{
			OrderNumber: OrderNumber("12345678903"),
			Status:      OrderStatus("PROCESSED"),
			Amount:      Amount(50050),
			UploadedAt:  fixedTime,
		}

		data, err := json.Marshal(order)
		require.NoError(t, err)

		// Проверяем наличие ключевых полей и отсутствие omitempty для ненулевого Amount
		assert.Contains(t, string(data), `"number":"12345678903"`)
		assert.Contains(t, string(data), `"status":"PROCESSED"`)
		assert.Contains(t, string(data), `"accrual":500.5`)
		assert.Contains(t, string(data), `"uploaded_at":"2024-03-23T12:00:00Z"`)
	})

	t.Run("order without accrual (omitempty)", func(t *testing.T) {
		order := Order{
			OrderNumber: OrderNumber("987654321"),
			Status:      OrderStatus("NEW"),
			Amount:      0, // Нулевое значение должно сработать для omitempty
			UploadedAt:  fixedTime,
		}

		data, err := json.Marshal(order)
		require.NoError(t, err)

		// Поле accrual не должно присутствовать в JSON, если оно 0 и помечено omitempty
		assert.NotContains(t, string(data), `"accrual"`)
		assert.Contains(t, string(data), `"status":"NEW"`)
	})

	t.Run("deserialization", func(t *testing.T) {
		jsonData := `{
			"number": "555",
			"status": "PROCESSING",
			"uploaded_at": "2024-03-23T12:00:00Z"
		}`

		var order Order
		err := json.Unmarshal([]byte(jsonData), &order)

		require.NoError(t, err)
		assert.Equal(t, OrderNumber("555"), order.OrderNumber)
		assert.Equal(t, OrderStatus("PROCESSING"), order.Status)
		assert.True(t, order.UploadedAt.Equal(fixedTime))
		assert.Equal(t, Amount(0), order.Amount)
	})
}
