package main

import (
	"net/http"
	"os"
	"testing"

	"gmart/internal/dto"

	"github.com/stretchr/testify/assert"
)

func TestFixEnv(t *testing.T) {
	// Очищаем окружение перед тестом
	os.Unsetenv("DATABASE_URI")
	os.Unsetenv("SERVICE_DATABASE_URI")

	t.Run("should map simple env to service prefix", func(t *testing.T) {
		val := "postgres://localhost:5432/test"
		os.Setenv("DATABASE_URI", val)
		defer os.Unsetenv("DATABASE_URI")
		defer os.Unsetenv("SERVICE_DATABASE_URI")

		FixEnv()

		assert.Equal(t, val, os.Getenv("SERVICE_DATABASE_URI"))
	})

	t.Run("should map camelCase field to snake_case env", func(t *testing.T) {
		val := "1h"
		// Поле JwtTTL -> JWT_TTL -> SERVICE_JWT_TTL
		os.Setenv("JWT_TTL", val)
		defer os.Unsetenv("JWT_TTL")
		defer os.Unsetenv("SERVICE_JWT_TTL")

		FixEnv()

		assert.Equal(t, val, os.Getenv("SERVICE_JWT_TTL"))
	})
}

func TestInitHuma_Structure(t *testing.T) {
	mux := http.NewServeMux()
	opts := &dto.CLIOptions{RunAddress: ":8080"}

	api := InitHuma(mux, opts)

	t.Run("api metadata", func(t *testing.T) {
		assert.NotNil(t, api)
		assert.Equal(t, "GopherMart API", api.OpenAPI().Info.Title)
		assert.Equal(t, "1.0.0", api.OpenAPI().Info.Version)
	})

	t.Run("security schemes", func(t *testing.T) {
		schemes := api.OpenAPI().Components.SecuritySchemes
		require, ok := schemes["bearer"]
		assert.True(t, ok, "bearer security scheme should exist")
		assert.Equal(t, "http", require.Type)
		assert.Equal(t, "bearer", require.Scheme)
		assert.Equal(t, "JWT", require.BearerFormat)
	})
}

func TestFixEnv_Comprehensive(t *testing.T) {
	// Список переменных для очистки после тестов
	envToCleanup := []string{
		"RUN_ADDRESS", "SERVICE_RUN_ADDRESS",
		"DATABASE_URI", "SERVICE_DATABASE_URI",
		"ACCRUAL_SYSTEM_ADDRESS", "SERVICE_ACCRUAL_SYSTEM_ADDRESS",
		"JWT_SECRET_KEY", "SERVICE_JWT_SECRET_KEY",
		"HTTP_READ_HEADER_TIMEOUT", "SERVICE_HTTP_READ_HEADER_TIMEOUT",
	}

	cleanup := func() {
		for _, env := range envToCleanup {
			os.Unsetenv(env)
		}
	}

	t.Run("basic mapping", func(t *testing.T) {
		cleanup()
		defer cleanup()

		os.Setenv("DATABASE_URI", "postgres://test")
		FixEnv()
		assert.Equal(t, "postgres://test", os.Getenv("SERVICE_DATABASE_URI"))
	})

	t.Run("mapping with snake_case conversion", func(t *testing.T) {
		cleanup()
		defer cleanup()

		// Проверяем сложные поля: HttpReadHeaderTimeout -> HTTP_READ_HEADER_TIMEOUT
		val := "10s"
		os.Setenv("HTTP_READ_HEADER_TIMEOUT", val)
		FixEnv()
		assert.Equal(t, val, os.Getenv("SERVICE_HTTP_READ_HEADER_TIMEOUT"))
	})

	t.Run("do not overwrite existing SERVICE_ env", func(t *testing.T) {
		cleanup()
		defer cleanup()

		os.Setenv("RUN_ADDRESS", ":8080")
		os.Setenv("SERVICE_RUN_ADDRESS", ":9090")

		FixEnv()

		// Значение не должно измениться на :8080
		assert.Equal(t, ":9090", os.Getenv("SERVICE_RUN_ADDRESS"))
	})

	t.Run("no env - no service env", func(t *testing.T) {
		cleanup()
		defer cleanup()

		FixEnv()
		assert.Empty(t, os.Getenv("SERVICE_ACCRUAL_SYSTEM_ADDRESS"))
	})
}
