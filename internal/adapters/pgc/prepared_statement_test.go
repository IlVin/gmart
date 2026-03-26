package pgc

import (
	"context"
	"testing"

	pgconn "github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	gomock "go.uber.org/mock/gomock"
)

type testUser struct {
	ID   int
	Name string
}

func userBinder(u *testUser) []any {
	return []any{&u.ID, &u.Name}
}

func TestPreparedStatement(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPg := NewMockPgInstance(ctrl)
	mockPool := NewMockPgxPoolIface(ctrl)
	ctx := context.Background()

	t.Run("FetchOne_Success", func(t *testing.T) {
		sql := "SELECT id, name FROM users WHERE id = $1"
		query := NewQuery(sql, userBinder)
		aq := query(mockPg)

		// Настраиваем цепочку: PgPool -> QueryRow -> Scan
		mockPg.EXPECT().
			PgPool(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, cb func(context.Context, PgxPoolIface) error) error {
				return cb(ctx, mockPool)
			})

		mockRow := NewMockRow(ctrl)
		mockPool.EXPECT().QueryRow(ctx, sql, 1).Return(mockRow)

		mockRow.EXPECT().Scan(gomock.Any()).DoAndReturn(func(dest ...any) error {
			*(dest[0].(*int)) = 1
			*(dest[1].(*string)) = "John"
			return nil
		})

		user, err := FetchOne(ctx, aq, 1)
		assert.NoError(t, err)
		assert.Equal(t, 1, user.ID)
		assert.Equal(t, "John", user.Name)
	})

	t.Run("QueryAll_Success", func(t *testing.T) {
		sql := "SELECT id, name FROM users"
		query := NewQuery(sql, userBinder)
		aq := query(mockPg)

		mockPg.EXPECT().
			PgPool(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, cb func(context.Context, PgxPoolIface) error) error {
				return cb(ctx, mockPool)
			})

		mockRows := NewMockRows(ctrl)
		mockPool.EXPECT().Query(ctx, sql).Return(mockRows, nil)

		// Имитируем 2 строки
		gomock.InOrder(
			mockRows.EXPECT().Next().Return(true),
			mockRows.EXPECT().Scan(gomock.Any()).DoAndReturn(func(dest ...any) error {
				*(dest[0].(*int)) = 1
				*(dest[1].(*string)) = "User1"
				return nil
			}),
			mockRows.EXPECT().Next().Return(true),
			mockRows.EXPECT().Scan(gomock.Any()).DoAndReturn(func(dest ...any) error {
				*(dest[0].(*int)) = 2
				*(dest[1].(*string)) = "User2"
				return nil
			}),
			mockRows.EXPECT().Next().Return(false),
		)
		mockRows.EXPECT().Err().Return(nil)
		mockRows.EXPECT().Close()

		var results []testUser
		for user, err := range QueryAll(ctx, aq) {
			assert.NoError(t, err)
			results = append(results, user)
		}

		assert.Len(t, results, 2)
		assert.Equal(t, "User2", results[1].Name)
	})

	t.Run("Exec_Success", func(t *testing.T) {
		sql := "UPDATE users SET name = $1"
		query := NewQuery(sql, userBinder)
		aq := query(mockPg)

		mockPg.EXPECT().
			PgPool(ctx, gomock.Any()).
			DoAndReturn(func(ctx context.Context, cb func(context.Context, PgxPoolIface) error) error {
				return cb(ctx, mockPool)
			})

		tag := pgconn.NewCommandTag("UPDATE 5")
		mockPool.EXPECT().Exec(ctx, sql, "NewName").Return(tag, nil)

		affected, err := Exec(ctx, aq, "NewName")
		assert.NoError(t, err)
		assert.Equal(t, int64(5), affected)
	})

	t.Run("ErrNilBinder", func(t *testing.T) {
		query := NewQuery[testUser]("SELECT 1", nil)
		aq := query(mockPg)

		_, err := FetchOne(ctx, aq)
		assert.ErrorIs(t, err, ErrNilBinder)
	})
}
