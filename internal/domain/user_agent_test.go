package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserAgent_String(t *testing.T) {
	tests := []struct {
		name  string
		input UserAgent
		want  string
	}{
		{
			name:  "chrome browser agent",
			input: UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
			want:  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		},
		{
			name:  "curl agent",
			input: UserAgent("curl/7.68.0"),
			want:  "curl/7.68.0",
		},
		{
			name:  "empty agent",
			input: UserAgent(""),
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Проверка метода String()
			assert.Equal(t, tt.want, tt.input.String())

			// Проверка явного приведения типа (используется при записи в БД или логи)
			assert.Equal(t, tt.want, string(tt.input))
		})
	}
}
