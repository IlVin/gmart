package loyalty

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"time"

	"gmart/internal/adapters/pgc"
	"gmart/internal/domain"

	"github.com/jackc/pgx/v5"
)

// SQL запросы
const sqlGetBalance = `
	SELECT (accrual - withdrawn) AS current, withdrawn
	FROM balances
	WHERE user_id = $1
`

// Список списаний от новых к старым
const sqlGetWithdrawals = `
	SELECT order_number, amount, processed_at
	FROM withdrawals
	WHERE user_id = $1
	ORDER BY processed_at DESC
`

// Списание баллов.
const sqlWithdraw = `
-- $1: user_id, $2: amount, $3: order_number, $4: now
	WITH
	-- 1. Проверяем наличие пользователя и достаточность средств
	check_funds AS (
		SELECT $1::bigint as user_id
		FROM (SELECT 1) AS dual
		WHERE (SELECT COALESCE(SUM(accrual - withdrawn), 0) FROM balances WHERE user_id = $1::bigint) >= $2
	),
	-- 2. Пробуем вставить списание. Вставляем только если check_funds вернул строку.
	new_withdrawal AS (
		INSERT INTO withdrawals (user_id, order_number, amount, processed_at)
		SELECT user_id, $3, $2, $4::timestamptz
		FROM check_funds
		ON CONFLICT (order_number) DO NOTHING
		RETURNING order_number
	),
	-- 3. Обновляем баланс только если запись в withdrawals успешно создана
	upd_balance AS (
		INSERT INTO balances (user_id, accrual, withdrawn, updated_at)
		SELECT $1::bigint, 0, $2, $4::timestamptz
		WHERE EXISTS (SELECT 1 FROM new_withdrawal)
		ON CONFLICT (user_id) DO UPDATE
		SET withdrawn = balances.withdrawn + EXCLUDED.withdrawn,
			updated_at = EXCLUDED.updated_at
		RETURNING user_id
	)
	-- 4. Определяем статус для возврата в Go
	SELECT
		CASE
			WHEN EXISTS (SELECT 1 FROM upd_balance) THEN 'success'
			WHEN NOT EXISTS (SELECT 1 FROM check_funds) THEN 'no_money'
			ELSE 'already_exists'
		END as result;
`

var (
	ErrInsufficientFunds = errors.New("insufficient balance")
	ErrWithdrawConflict  = errors.New("withdrawal for this order already exists")
	ErrEmpty             = errors.New("empty result")
)

//go:generate $GOPATH/bin/mockgen -source=$GOFILE                              -destination=repository_mock_test.go  -package=loyalty
//go:generate $GOPATH/bin/mockgen -source=../../adapters/pgc/pg_instance.go    -destination=pg_instance_mock_test.go -package=loyalty
//go:generate $GOPATH/bin/mockgen                                              -destination=pgx_mock_test.go         -package=loyalty github.com/jackc/pgx/v5 Tx,Row,BatchResults,Rows

type LoyaltyMetrics interface {
	ObserveDB(op domain.OpType, duration time.Duration)
	IncWithdrawal(status string) // success, insufficient_funds, conflict
	ObserveWithdrawalAmount(amount domain.Amount)
}

type LoyaltyRepo struct {
	pg      pgc.PgInstance
	metrics LoyaltyMetrics
	now     func() time.Time
}

func NewLoyaltyRepo(pg pgc.PgInstance, m LoyaltyMetrics) *LoyaltyRepo {
	return &LoyaltyRepo{
		pg:      pg,
		metrics: m,
		now:     time.Now,
	}
}

// GetBalance возвращает текущий остаток и общую сумму списаний
func (r *LoyaltyRepo) GetBalance(ctx context.Context, userID domain.UserID) (current domain.Amount, withdrawn domain.Amount, err error) {
	start := r.now()
	defer func() {
		if r.metrics != nil {
			r.metrics.ObserveDB(domain.OpQuery, time.Since(start))
		}
	}()

	err = r.pg.PgPool(ctx, func(ctx context.Context, pool pgc.PgxPoolIface) error {
		err := pool.QueryRow(ctx, sqlGetBalance, userID).Scan(&current, &withdrawn)
		if errors.Is(err, pgx.ErrNoRows) {
			current, withdrawn = 0, 0
			return nil
		}
		return err
	})

	if err != nil {
		slog.Error("db query failed", "op", "LoyaltyRepo.GetBalance", "err", err, "user_id", userID)
		return 0, 0, fmt.Errorf("get balance fail: %w", err)
	}

	return current, withdrawn, nil
}

// Withdraw регистрирует списание баллов
func (r *LoyaltyRepo) Withdraw(ctx context.Context, userID domain.UserID, order domain.OrderNumber, amount domain.Amount) error {
	start := r.now()
	defer func() {
		if r.metrics != nil {
			r.metrics.ObserveDB(domain.OpQuery, time.Since(start))
		}
	}()

	var status string
	err := r.pg.PgPool(ctx, func(ctx context.Context, pool pgc.PgxPoolIface) error {
		return pool.QueryRow(ctx, sqlWithdraw, userID, amount, order, r.now()).Scan(&status)
	})

	if err != nil {
		slog.Error("db query failed",
			slog.String("op", "LoyaltyRepo.Withdraw"),
			slog.Any("err", err),
			slog.String("user_id", userID.String()),
			slog.String("order", order.String()),
		)
		return fmt.Errorf("withdraw database error: %w", err)
	}

	// Обрабатываем бизнес-логику на основе статуса из БД
	switch status {
	case "success":
		if r.metrics != nil {
			r.metrics.IncWithdrawal("success")
			r.metrics.ObserveWithdrawalAmount(amount)
		}
		return nil

	case "no_money":
		if r.metrics != nil {
			r.metrics.IncWithdrawal("insufficient_funds")
		}
		return ErrInsufficientFunds

	case "already_exists":
		if r.metrics != nil {
			r.metrics.IncWithdrawal("conflict")
		}
		return ErrWithdrawConflict

	default:
		slog.Error("unexpected withdraw status", "status", status, "user_id", userID)
		return fmt.Errorf("unexpected withdraw status: %s", status)
	}
}

// GetWithdrawals возвращает историю списаний пользователя
func (r *LoyaltyRepo) GetWithdrawals(ctx context.Context, userID domain.UserID) iter.Seq2[domain.Withdrawal, error] {

	return func(yield func(domain.Withdrawal, error) bool) {
		// yield(v, err) bool отправляет данные в цикл.

		start := r.now()

		var rows pgx.Rows
		var err error
		err = r.pg.PgPool(ctx, func(ctx context.Context, pool pgc.PgxPoolIface) error {
			rows, err = pool.Query(ctx, sqlGetWithdrawals, userID)
			return err // Возвращаем ошибку для сбора статистики драйвером PgInstance
		})
		defer rows.Close()

		// Собираем метрики, но это уже будет не все время запрос + доставка данных, а только время работы запроса.
		if r.metrics != nil {
			r.metrics.ObserveDB(domain.OpQuery, time.Since(start))
		}

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				yield(domain.Withdrawal{}, ErrEmpty) // Не смотрим на возвращенное значение yield - все равно заканчиваем итерацию
				return
			}
			yield(domain.Withdrawal{}, err) // Не смотрим на возвращенное значение yield - все равно заканчиваем итерацию
			return
		}

		for rows.Next() {
			var item domain.Withdrawal
			if ctx.Err() != nil {
				yield(domain.Withdrawal{}, ctx.Err()) // Не смотрим на возвращенное значение yield - все равно заканчиваем итерацию
				return
			}
			if err := rows.Scan(&item.OrderNumber, &item.Amount, &item.ProcessedAt); err != nil {
				yield(domain.Withdrawal{}, err) // Не смотрим на возвращенное значение yield - все равно заканчиваем итерацию
				return
			}
			// Если yield вернул false (например, в цикле вызвали break), прекращаем работу.
			if !yield(item, nil) {
				return
			}
		}

		if err := rows.Err(); err != nil {
			yield(domain.Withdrawal{}, err)
		}
	}

}
