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

func TestInitHuma(t *testing.T) {
	mux := http.NewServeMux()
	opts := &dto.CLIOptions{
		RunAddress: ":8080",
	}

	api := InitHuma(mux, opts)

	assert.NotNil(t, api)
	assert.Equal(t, "GopherMart API", api.OpenAPI().Info.Title)

	// В Huma v2 SecuritySchemes — это map[string]*SecurityScheme
	// Поля доступны напрямую без .Value
	assert.Contains(t, api.OpenAPI().Components.SecuritySchemes, "bearer")
	scheme := api.OpenAPI().Components.SecuritySchemes["bearer"]

	assert.Equal(t, "http", scheme.Type)
	assert.Equal(t, "bearer", scheme.Scheme)
}
