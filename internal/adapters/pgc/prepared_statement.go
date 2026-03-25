package pgc

import (
	"context"
	"errors"
	"iter"
	"log/slog"

	pgx "github.com/jackc/pgx/v5"
)

var (
	ErrNilBinder = errors.New("binder is required for this operation")
)

//go:generate $GOPATH/bin/mockgen -source=$GOFILE -destination=prepared_statement_mock_test.go  -package=pgc

// Binder — функция, описывающая маппинг полей структуры T при сканировании.
type Binder[T any] func(*T) []any

// Query — интерфейс описания запроса (чертеж).
type Query[T any] interface {
	Ctx(ctx context.Context, pg PgInstance) Executor[T]
}

// Executor — интерфейс для выполнения привязанного к ресурсам запроса.
type Executor[T any] interface {
	All(args ...any) iter.Seq2[T, error]
	One(args ...any) (T, error)
	Exec(args ...any) (int64, error)
}

// NewQuery создает новое типизированное описание SQL-запроса.
// Это "холодный" объект, он не содержит соединений и безопасен для глобального использования.
func NewQuery[T any](sql string, binder Binder[T]) Query[T] {
	if sql == "" {
		panic("pgc: sql query cannot be empty")
	}
	return &queryDef[T]{
		sql:    sql,
		binder: binder,
	}
}

// --- Внутренняя реализация ---

type queryDef[T any] struct {
	sql    string
	binder Binder[T]
}

// Ctx — метод-мост, который привязывает запрос к ресурсам
func (qd *queryDef[T]) Ctx(ctx context.Context, pg PgInstance) Executor[T] {
	return &boundQuery[T]{
		qd:  qd,
		ctx: ctx,
		pg:  pg,
	}
}

type boundQuery[T any] struct {
	qd  *queryDef[T]
	ctx context.Context
	pg  PgInstance
}

// All выполняет запрос и возвращает итератор.
func (bq *boundQuery[T]) All(args ...any) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		var zero T
		if bq.qd.binder == nil {
			slog.Error("pgc: call All() on query without binder", "sql", bq.qd.sql)
			yield(zero, ErrNilBinder)
			return
		}
		rows, err := bq.pg.Query(bq.ctx, bq.qd.sql, args...)
		if err != nil {
			yield(zero, err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var item T
			if err := rows.Scan(bq.qd.binder(&item)...); err != nil {
				if !yield(zero, err) {
					return
				}
				return
			}
			if !yield(item, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(zero, err)
		}
	}
}

// One выполняет запрос и возвращает одну строку.
func (bq *boundQuery[T]) One(args ...any) (T, error) {
	var item T
	if bq.qd.binder == nil {
		slog.Error("pgc: call One() on query without binder", "sql", bq.qd.sql)
		return item, ErrNilBinder
	}
	row, err := bq.pg.Fetch(bq.ctx, bq.qd.sql, args...)
	if err != nil {
		return item, err
	}
	if row == nil {
		return item, pgx.ErrNoRows
	}
	if err := row.Scan(bq.qd.binder(&item)...); err != nil {
		return item, err
	}
	return item, nil
}

// Exec исполняет INSERT/UPDATE/DELETE и возвращает кол-во затронутых строк
func (bq *boundQuery[T]) Exec(args ...any) (int64, error) {
	tag, err := bq.pg.Exec(bq.ctx, bq.qd.sql, args...)
	return tag.RowsAffected(), err
}
