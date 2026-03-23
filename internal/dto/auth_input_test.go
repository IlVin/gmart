package dto

import (
	"gmart/internal/domain"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestAuthInput_Resolve(t *testing.T) {
	validSession := domain.SessionID(uuid.New())
	emptySession := domain.SessionID(uuid.Nil)

	tests := []struct {
		name    string
		input   AuthInput
		wantErr bool
	}{
		{
			name: "valid with both header and cookie",
			input: AuthInput{
				Authorization: "Bearer token",
				SessionID:     validSession,
			},
			wantErr: false,
		},
		{
			name: "valid with only authorization header",
			input: AuthInput{
				Authorization: "Bearer token",
				SessionID:     emptySession,
			},
			wantErr: false,
		},
		{
			name: "valid with only session_id cookie",
			input: AuthInput{
				Authorization: "",
				SessionID:     validSession,
			},
			wantErr: false,
		},
		{
			name: "invalid - both missing",
			input: AuthInput{
				Authorization: "",
				SessionID:     emptySession,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Передаем nil, так как текущий Resolve не использует ctx и prefix
			errs := tt.input.Resolve(nil, "")

			if tt.wantErr {
				// Сначала проверяем, что ошибки вообще есть
				assert.NotEmpty(t, errs, "Должна быть ошибка, если авторизация отсутствует")
				if len(errs) > 0 {
					assert.Contains(t, errs[0].Error(), "Missing Authorization")
				}
			} else {
				assert.Empty(t, errs, "Ошибок быть не должно")
			}
		})
	}
}
