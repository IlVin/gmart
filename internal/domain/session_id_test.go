package domain

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestSessionID_ParseSessionID(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"
	invalidUUID := "not-a-uuid"

	t.Run("valid uuid string", func(t *testing.T) {
		id, err := ParseSessionID(validUUID)
		assert.NoError(t, err)
		assert.Equal(t, validUUID, id.String())
	})

	t.Run("invalid uuid string", func(t *testing.T) {
		id, err := ParseSessionID(invalidUUID)
		assert.Error(t, err)
		assert.Equal(t, SessionID(uuid.Nil), id)
	})
}

func TestSessionID_Methods(t *testing.T) {
	raw := uuid.New()
	sessionID := SessionID(raw)

	t.Run("String() method", func(t *testing.T) {
		assert.Equal(t, raw.String(), sessionID.String())
	})

	t.Run("UUID() method", func(t *testing.T) {
		assert.Equal(t, raw, sessionID.UUID())
	})
}

func TestSessionID_UnmarshalText(t *testing.T) {
	validUUID := "550e8400-e29b-41d4-a716-446655440000"

	t.Run("unmarshal valid bytes", func(t *testing.T) {
		var s SessionID
		err := s.UnmarshalText([]byte(validUUID))
		assert.NoError(t, err)
		assert.Equal(t, validUUID, s.String())
	})

	t.Run("unmarshal empty bytes", func(t *testing.T) {
		var s SessionID
		// Устанавливаем ненулевое значение, чтобы проверить зануление
		s = SessionID(uuid.New())

		err := s.UnmarshalText([]byte(""))
		assert.NoError(t, err)
		assert.Equal(t, SessionID(uuid.Nil), s)
	})

	t.Run("unmarshal invalid bytes", func(t *testing.T) {
		var s SessionID
		err := s.UnmarshalText([]byte("wrong-format"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid session id format")
	})
}
