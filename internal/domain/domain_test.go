package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBalance_Creation(t *testing.T) {
	t.Run("should create balance with correct values", func(t *testing.T) {
		// Представим, что Amount — это float64 или int64 (исходя из логики Gophermart)
		curr := Amount(50050)
		withdr := Amount(10000)

		b := Balance{
			Current:   curr,
			Withdrawn: withdr,
		}

		assert.Equal(t, curr, b.Current, "Current amount should match")
		assert.Equal(t, withdr, b.Withdrawn, "Withdrawn amount should match")
	})

	t.Run("zero value balance", func(t *testing.T) {
		var b Balance

		assert.Equal(t, Amount(0), b.Current)
		assert.Equal(t, Amount(0), b.Withdrawn)
	})
}
