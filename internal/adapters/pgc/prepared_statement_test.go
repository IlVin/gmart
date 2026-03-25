package pgc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

type TestModel struct {
	ID   int
	Name string
}

func TestPreparedStatement_All(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockRows := NewMockRows(ctrl) // Предполагаем, что мок для pgx.Rows сгенерирован

	binder := func(m *TestModel) []any {
		return []any{&m.ID, &m.Name}
	}

	ctx := context.Background()
	sql := "SELECT id, name FROM users"
	ps := NewStatement(mockPg, sql, binder)

	t.Run("success iteration", func(t *testing.T) {
		mockPg.EXPECT().Query(ctx, sql).Return(mockRows, nil)

		// Настраиваем поведение rows для 2 итераций
		gomock.InOrder(
			mockRows.EXPECT().Next().Return(true),
			mockRows.EXPECT().Scan(gomock.Any(), gomock.Any()).DoAndReturn(func(dest ...any) error {
				*dest[0].(*int) = 1
				*dest[1].(*string) = "first"
				return nil
			}),
			mockRows.EXPECT().Next().Return(true),
			mockRows.EXPECT().Scan(gomock.Any(), gomock.Any()).DoAndReturn(func(dest ...any) error {
				*dest[0].(*int) = 2
				*dest[1].(*string) = "second"
				return nil
			}),
			mockRows.EXPECT().Next().Return(false),
			mockRows.EXPECT().Err().Return(nil),
			mockRows.EXPECT().Close(),
		)

		var results []TestModel
		for item, err := range ps.All(ctx) {
			require.NoError(t, err)
			results = append(results, item)
		}

		assert.Len(t, results, 2)
		assert.Equal(t, "first", results[0].Name)
	})

	t.Run("early break (leak check)", func(t *testing.T) {
		mockPg.EXPECT().Query(ctx, sql).Return(mockRows, nil)

		// Даже если мы выходим из цикла раньше, Close() должен быть вызван
		mockRows.EXPECT().Next().Return(true)
		mockRows.EXPECT().Scan(gomock.Any(), gomock.Any()).Return(nil)
		mockRows.EXPECT().Close() // Ожидаем вызов из-за defer

		for _, _ = range ps.All(ctx) {
			break // Досрочный выход
		}
	})
}

func TestPreparedStatement_One(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockRow := NewMockRows(ctrl) // Мок для pgx.Row

	binder := func(m *TestModel) []any {
		return []any{&m.ID}
	}

	ps := NewStatement(mockPg, "SELECT id", binder)
	ctx := context.Background()

	t.Run("success one", func(t *testing.T) {
		mockPg.EXPECT().Fetch(ctx, gomock.Any()).Return(mockRow, nil)
		mockRow.EXPECT().Scan(gomock.Any()).DoAndReturn(func(dest ...any) error {
			*dest[0].(*int) = 100
			return nil
		})

		item, err := ps.One(ctx)
		assert.NoError(t, err)
		assert.Equal(t, 100, item.ID)
	})

	t.Run("fetch error", func(t *testing.T) {
		mockPg.EXPECT().Fetch(ctx, gomock.Any()).Return(nil, errors.New("db error"))

		_, err := ps.One(ctx)
		assert.Error(t, err)
	})
}
