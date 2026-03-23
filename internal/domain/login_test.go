package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLogin_String(t *testing.T) {
	tests := []struct {
		name  string
		input Login
		want  string
	}{
		{
			name:  "simple login",
			input: Login("admin"),
			want:  "admin",
		},
		{
			name:  "empty login",
			input: Login(""),
			want:  "",
		},
		{
			name:  "login with special characters",
			input: Login("user_123@host"),
			want:  "user_123@host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Проверяем метод String()
			assert.Equal(t, tt.want, tt.input.String())

			// Проверяем неявное приведение к строке (например, в fmt.Sprintf)
			assert.Equal(t, tt.want, string(tt.input))
		})
	}
}
