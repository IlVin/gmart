package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPassword_String(t *testing.T) {
	tests := []struct {
		name  string
		input Password
		want  string
	}{
		{
			name:  "simple password",
			input: Password("qwerty12345"),
			want:  "qwerty12345",
		},
		{
			name:  "empty password",
			input: Password(""),
			want:  "",
		},
		{
			name:  "password with special symbols",
			input: Password("!@#$%^&*()_+ "),
			want:  "!@#$%^&*()_+ ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Проверяем явный метод String()
			assert.Equal(t, tt.want, tt.input.String())

			// Проверяем базовое приведение типа (используется при передаче в bcrypt)
			assert.Equal(t, tt.want, string(tt.input))
		})
	}
}
