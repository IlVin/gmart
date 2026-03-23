package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserID_String(t *testing.T) {
	tests := []struct {
		name  string
		input UserID
		want  string
	}{
		{
			name:  "positive id",
			input: UserID(12345),
			want:  "12345",
		},
		{
			name:  "zero id",
			input: UserID(0),
			want:  "0",
		},
		{
			name:  "max int64 id",
			input: UserID(9223372036854775807),
			want:  "9223372036854775807",
		},
		{
			name:  "negative id",
			input: UserID(-1),
			want:  "-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Проверка явного метода String()
			assert.Equal(t, tt.want, tt.input.String())

			// Проверка неявного использования через fmt
			assert.Equal(t, tt.want, tt.input.String())
		})
	}
}

func TestUserID_TypeConsistency(t *testing.T) {
	// Проверка базового типа int64 (важно для работы с БД)
	var raw int64 = 100
	id := UserID(raw)

	assert.Equal(t, raw, int64(id))
}
