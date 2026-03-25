package pgc

import (
	"context"
	"iter"
)

// Binder — функция, которая говорит, в какие поля структуры сканировать данные
type Binder[T any] func(*T) []any

// PreparedStatement — типизированная обертка над SQL-запросом
type PreparedStatement[T any] struct {
	pg     PgInstance
	sql    string
	binder Binder[T]
}

// NewStatement создает новый подготовленный запрос
func NewStatement[T any](pg PgInstance, sql string, binder Binder[T]) *PreparedStatement[T] {
	return &PreparedStatement[T]{
		pg:     pg,
		sql:    sql,
		binder: binder,
	}
}

// All возвращает итератор (iter.Seq2) для получения всех строк.
// Автоматически закрывает rows при выходе из цикла for-range.
func (ps *PreparedStatement[T]) All(ctx context.Context, args ...any) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T

		rows, err := ps.pg.Query(ctx, ps.sql, args...)
		if err != nil {
			yield(zero, err)
			return
		}
		defer rows.Close() // ГАРАНТИЯ: соединение вернется в пул при выходе из итератора

		for rows.Next() {
			var item T
			dest := ps.binder(&item)

			if err := rows.Scan(dest...); err != nil {
				yield(zero, err)
				return
			}

			// Передаем объект в цикл. Если там break — yield вернет false и мы выйдем.
			if !yield(item, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(zero, err)
		}
	}
}

// One возвращает ровно одну строку
func (ps *PreparedStatement[T]) One(ctx context.Context, args ...any) (T, error) {
	var item T
	row, err := ps.pg.Fetch(ctx, ps.sql, args...)
	if err != nil {
		return item, err
	}

	if err := row.Scan(ps.binder(&item)...); err != nil {
		return item, err
	}

	return item, nil
}
