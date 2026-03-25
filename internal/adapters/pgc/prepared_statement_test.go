package pgc

import (
	"context"
	"testing"

	pgconn "github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"
)

type TestModel struct {
	ID   int
	Name string
}

func TestQuery_Workflow(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockRows := NewMockRows(ctrl)
	mockRow := NewMockRow(ctrl)

	ctx := context.Background()
	sql := "SELECT id, name FROM users"

	binder := func(m *TestModel) []any {
		return []any{&m.ID, &m.Name}
	}

	// Создаем наше описание запроса
	q := NewQuery(sql, binder)

	t.Run("success_all_iteration", func(t *testing.T) {
		mockPg.EXPECT().Query(ctx, sql).Return(mockRows, nil)

		gomock.InOrder(
			mockRows.EXPECT().Next().Return(true),
			mockRows.EXPECT().Scan(gomock.Any(), gomock.Any()).DoAndReturn(func(dest ...any) error {
				*dest[0].(*int) = 1
				*dest[1].(*string) = "first"
				return nil
			}),
			mockRows.EXPECT().Next().Return(false),
			mockRows.EXPECT().Err().Return(nil),
			mockRows.EXPECT().Close(),
		)

		var results []TestModel
		// Используем новый синтаксис .Ctx().All()
		for item, err := range q.Ctx(ctx, mockPg).All() {
			require.NoError(t, err)
			results = append(results, item)
		}

		assert.Len(t, results, 1)
		assert.Equal(t, "first", results[0].Name)
	})

	t.Run("one_success", func(t *testing.T) {
		mockPg.EXPECT().Fetch(ctx, sql).Return(mockRow, nil)
		mockRow.EXPECT().Scan(gomock.Any(), gomock.Any()).DoAndReturn(func(dest ...any) error {
			*dest[0].(*int) = 100
			return nil
		})

		item, err := q.Ctx(ctx, mockPg).One()
		assert.NoError(t, err)
		assert.Equal(t, 100, item.ID)
	})

	t.Run("exec_success", func(t *testing.T) {
		deleteSQL := "DELETE FROM users"
		// Для Exec биндер может быть nil
		qExec := NewQuery(deleteSQL, Binder[struct{}](nil))

		tag := pgconn.NewCommandTag("DELETE 5")
		mockPg.EXPECT().Exec(ctx, deleteSQL).Return(tag, nil)

		affected, err := qExec.Ctx(ctx, mockPg).Exec()
		assert.NoError(t, err)
		assert.Equal(t, int64(5), affected)
	})

	t.Run("nil_binder_error", func(t *testing.T) {
		// Создаем запрос без биндера специально для теста ошибки
		qBad := NewQuery("SELECT 1", Binder[TestModel](nil))

		// Тестируем One
		_, err := qBad.Ctx(ctx, mockPg).One()
		assert.ErrorIs(t, err, ErrNilBinder)

		// Тестируем All
		for _, err := range qBad.Ctx(ctx, mockPg).All() {
			assert.ErrorIs(t, err, ErrNilBinder)
		}
	})

	t.Run("early_break_leak_check", func(t *testing.T) {
		mockPg.EXPECT().Query(ctx, sql).Return(mockRows, nil)
		mockRows.EXPECT().Next().Return(true)
		mockRows.EXPECT().Scan(gomock.Any(), gomock.Any()).Return(nil)
		mockRows.EXPECT().Close() // Должен вызваться при break

		for _, _ = range q.Ctx(ctx, mockPg).All() {
			break
		}
	})

	t.Run("empty_sql_panic", func(t *testing.T) {
		assert.Panics(t, func() {
			NewQuery("", binder)
		})
	})
}
