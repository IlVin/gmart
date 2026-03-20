package dto

import (
	"gmart/internal/domain"

	"github.com/danielgtaylor/huma/v2"
)

// --- DTO ---
type AuthInput struct {
	Authorization string           `header:"Authorization" doc:"Bearer <token>"`
	SessionID     domain.SessionID `cookie:"session_id" doc:"Cookie session_id"`
}

// Реализуем Resolver, чтобы проверять токен и session_id прямо при парсинге входа
func (i *AuthInput) Resolve(ctx huma.Context, prefix string) []error {
	if i.Authorization == "" && i.SessionID.String() == "" {
		return []error{huma.Error401Unauthorized("Missing Authorization header and session_id cookie")}
	}

	return nil
}
