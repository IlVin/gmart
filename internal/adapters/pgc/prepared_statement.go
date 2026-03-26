package pgc

import (
	"context"
	"errors"
	"iter"
)

var (
	ErrNilBinder = errors.New("binder is required for this operation")
)

//go:generate $GOPATH/bin/mockgen -source=$GOFILE -destination=prepared_statement_mock_test.go  -package=pgc

// Binder — функция, описывающая маппинг полей структуры T при сканировании.
type Binder[T any] func(*T) []any

// ActiveQuery объединяет описание запроса с живыми ресурсами (ctx, pg).
type ActiveQuery[T any] struct {
	pg     PgInstance
	sql    string
	binder Binder[T]
}

// Query — это функция-активатор. При вызове q(ctx, pg) возвращает готовый к работе запрос.
type Query[T any] func(pg PgInstance) *ActiveQuery[T]

// NewQuery создает описание запроса.
func NewQuery[T any](sql string, binder Binder[T]) Query[T] {
	if sql == "" {
		panic("pgc: sql query cannot be empty")
	}
	return func(pg PgInstance) *ActiveQuery[T] {
		return &ActiveQuery[T]{
			pg:     pg,
			sql:    sql,
			binder: binder,
		}
	}
}

// QueryAll выполняет запрос и возвращает итератор Go 1.23.
func QueryAll[T any](ctx context.Context, aq *ActiveQuery[T], args ...any) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		if aq.binder == nil {
			yield(zero, ErrNilBinder)
			return
		}

		err := aq.pg.PgPool(ctx, func(pCtx context.Context, pool PgxPoolIface) error {
			rows, err := pool.Query(pCtx, aq.sql, args...)
			if err != nil {
				return err
			}
			defer rows.Close()

			for rows.Next() {
				var item T
				if err := rows.Scan(aq.binder(&item)...); err != nil {
					return err
				}
				if !yield(item, nil) {
					return nil // Пользователь прервал цикл (break)
				}
			}
			return rows.Err()
		})

		if err != nil {
			yield(zero, err)
		}
	}
}

// FetchOne выполняет запрос и возвращает одну строку.
func FetchOne[T any](ctx context.Context, aq *ActiveQuery[T], args ...any) (T, error) {
	var item T
	if aq.binder == nil {
		return item, ErrNilBinder
	}

	err := aq.pg.PgPool(ctx, func(pCtx context.Context, pool PgxPoolIface) error {
		return pool.QueryRow(pCtx, aq.sql, args...).Scan(aq.binder(&item)...)
	})

	return item, err
}

// Exec исполняет команду и возвращает количество затронутых строк.
func Exec[T any](ctx context.Context, aq *ActiveQuery[T], args ...any) (int64, error) {
	var rowsAffected int64
	err := aq.pg.PgPool(ctx, func(pCtx context.Context, pool PgxPoolIface) error {
		tag, err := pool.Exec(pCtx, aq.sql, args...)
		if err != nil {
			return err
		}
		rowsAffected = tag.RowsAffected()
		return nil
	})
	return rowsAffected, err
}
