package domain

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithdrawal_JSON(t *testing.T) {
	fixedTime := time.Date(2024, 3, 23, 15, 0, 0, 0, time.UTC)

	t.Run("serialization", func(t *testing.T) {
		w := Withdrawal{
			OrderNumber: OrderNumber("23772256761"),
			Amount:      Amount(10050),
			ProcessedAt: fixedTime,
		}

		data, err := json.Marshal(w)
		require.NoError(t, err)

		// Проверяем соответствие именования полей JSON-тегам
		assert.Contains(t, string(data), `"order":"23772256761"`)
		assert.Contains(t, string(data), `"sum":100.5`)
		assert.Contains(t, string(data), `"processed_at":"2024-03-23T15:00:00Z"`)
	})

	t.Run("deserialization", func(t *testing.T) {
		jsonData := `{
			"order": "7029352454",
			"sum": 450,
			"processed_at": "2024-03-23T15:00:00Z"
		}`

		var w Withdrawal
		err := json.Unmarshal([]byte(jsonData), &w)

		require.NoError(t, err)
		assert.Equal(t, OrderNumber("7029352454"), w.OrderNumber)
		assert.Equal(t, Amount(45000), w.Amount)
		assert.True(t, w.ProcessedAt.Equal(fixedTime))
	})
}
