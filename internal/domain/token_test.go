package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToken_String(t *testing.T) {
	tests := []struct {
		name  string
		input Token
		want  string
	}{
		{
			name:  "standard jwt token",
			input: Token("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."),
			want:  "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
		},
		{
			name:  "empty token",
			input: Token(""),
			want:  "",
		},
		{
			name:  "bearer prefixed content",
			input: Token("bearer secret_content"),
			want:  "bearer secret_content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Проверка метода String()
			assert.Equal(t, tt.want, tt.input.String())

			// Проверка базового приведения типа string(t)
			assert.Equal(t, tt.want, string(tt.input))
		})
	}
}
