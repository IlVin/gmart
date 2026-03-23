package dto

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCLIOptions_Values(t *testing.T) {
	t.Run("manual assignment consistency", func(t *testing.T) {
		// Проверяем, что все типы данных в структуре ведут себя ожидаемо
		opts := CLIOptions{
			RunAddress:            ":9090",
			DatabaseURI:           "postgres://localhost:5432",
			AccrualSystemAddress:  "http://accrual:8080",
			JwtSecretKey:          "secret",
			JwtTTL:                24 * time.Hour,
			SessionTTL:            720 * time.Hour,
			RunMigrations:         true,
			HttpReadHeaderTimeout: 10 * time.Second,
			HttpIdleTimeout:       60 * time.Second,
		}

		assert.Equal(t, ":9090", opts.RunAddress)
		assert.Equal(t, 24*time.Hour, opts.JwtTTL)
		assert.True(t, opts.RunMigrations)
		assert.Equal(t, 10*time.Second, opts.HttpReadHeaderTimeout)
	})

	t.Run("time duration parsing logic", func(t *testing.T) {
		// Эмулируем парсинг строки в duration, как это делает humacli/env
		rawTTL := "15m"
		duration, err := time.ParseDuration(rawTTL)

		assert.NoError(t, err)
		opts := CLIOptions{JwtTTL: duration}
		assert.Equal(t, 15*time.Minute, opts.JwtTTL)
	})
}
