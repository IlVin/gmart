package dto

import (
	"encoding/json"
	"testing"
	"time"

	"gmart/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccrualResponse_JSON(t *testing.T) {
	t.Run("full response serialization", func(t *testing.T) {
		res := AccrualResponse{
			Order:      domain.OrderNumber("12345"),
			Status:     domain.OrderStatus("PROCESSED"),
			Accrual:    domain.Amount(15050),
			RetryAfter: 5 * time.Second,
		}

		data, err := json.Marshal(res)
		require.NoError(t, err)

		jsonStr := string(data)
		// Добавлен t первым аргументом во все вызовы assert
		assert.Contains(t, jsonStr, `"order":"12345"`)
		assert.Contains(t, jsonStr, `"status":"PROCESSED"`)
		assert.Contains(t, jsonStr, `"accrual":150.5`)
		assert.NotContains(t, jsonStr, "RetryAfter")
		assert.NotContains(t, jsonStr, "retry_after")
	})

	t.Run("empty accrual omitempty", func(t *testing.T) {
		res := AccrualResponse{
			Order:  domain.OrderNumber("67890"),
			Status: domain.OrderStatus("PROCESSING"),
		}

		data, err := json.Marshal(res)
		require.NoError(t, err)

		assert.NotContains(t, string(data), `"accrual"`)
	})

	t.Run("deserialization from accrual system", func(t *testing.T) {
		jsonData := `{
			"order": "777",
			"status": "REGISTERED"
		}`

		var res AccrualResponse
		err := json.Unmarshal([]byte(jsonData), &res)

		require.NoError(t, err)
		assert.Equal(t, domain.OrderNumber("777"), res.Order)
		assert.Equal(t, domain.OrderStatus("REGISTERED"), res.Status)
		assert.Equal(t, domain.Amount(0), res.Accrual)
	})
}
