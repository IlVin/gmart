package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrderNumber_String(t *testing.T) {
	tests := []struct {
		name  string
		input OrderNumber
		want  string
	}{
		{
			name:  "valid numeric order number",
			input: OrderNumber("1234567890"),
			want:  "1234567890",
		},
		{
			name:  "empty order number",
			input: OrderNumber(""),
			want:  "",
		},
		{
			name:  "short order number",
			input: OrderNumber("777"),
			want:  "777",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Проверяем метод String()
			assert.Equal(t, tt.want, tt.input.String())

			// Проверяем возможность приведения к базовому типу string
			assert.Equal(t, tt.want, string(tt.input))
		})
	}
}
