package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBalance_JSON(t *testing.T) {
	t.Run("full_balance_marshaling", func(t *testing.T) {
		b := Balance{
			Current:   NewAmountFromRubles(100.50),
			Withdrawn: NewAmountFromRubles(42.00),
		}

		data, err := json.Marshal(b)
		require.NoError(t, err)

		// Проверяем, что оба поля присутствуют
		assert.JSONEq(t, `{"current":100.5, "withdrawn":42}`, string(data))
	})

	t.Run("omitzero_logic", func(t *testing.T) {
		// Создаем баланс с нулевыми значениями
		b := Balance{
			Current:   Amount(0),
			Withdrawn: Amount(0),
		}

		data, err := json.Marshal(b)
		require.NoError(t, err)

		// Благодаря тегу omitzero (Go 1.24) или методу IsZero,
		// пустые поля должны исчезнуть из JSON
		assert.JSONEq(t, `{}`, string(data), "Zero amounts should be omitted from JSON")
	})

	t.Run("partial_omitzero", func(t *testing.T) {
		b := Balance{
			Current:   Amount(500),
			Withdrawn: Amount(0),
		}

		data, err := json.Marshal(b)
		require.NoError(t, err)

		// Только ненулевое поле должно остаться
		assert.JSONEq(t, `{"current":500}`, string(data))
	})
}

func TestBalance_IsZero_Integration(t *testing.T) {
	t.Run("check_amount_is_zero", func(t *testing.T) {
		zero := Amount(0)
		notZero := NewAmountFromRubles(0.01)

		assert.True(t, zero.IsZero())
		assert.False(t, notZero.IsZero())
	})
}
