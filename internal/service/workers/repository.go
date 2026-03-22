package workers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gmart/internal/adapters/metrics"
	"gmart/internal/adapters/pgc"
	"gmart/internal/domain"

	"github.com/jackc/pgx/v5"
)

const sqlAcquireNextOrder = `
	UPDATE orders
	SET accrualed_at = $1
	WHERE order_number = (
		SELECT order_number
		FROM orders
		WHERE status IN ('NEW', 'PROCESSING', 'REGISTERED')
		  AND (accrualed_at IS NULL OR accrualed_at < $1::timestamptz - INTERVAL '120 seconds')
		ORDER BY uploaded_at ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	)
	RETURNING order_number, status;
`

const sqlUpdateOrderAndBalance = `
-- $1: order_number, $2: status, $3: accrual, $4: now
	WITH updated_order AS (
		UPDATE orders
		SET status = $2, accrual = $3, accrualed_at = $4
		WHERE order_number = $1
			AND status NOT IN ('PROCESSED', 'INVALID')
			AND accrual = 0
		RETURNING user_id, status, accrual
	)
	INSERT INTO balances (user_id, accrual, withdrawn, updated_at)
	SELECT user_id, accrual, 0, $4
	FROM updated_order
	WHERE status = 'PROCESSED' AND accrual > 0
	ON CONFLICT (user_id) DO UPDATE
	SET accrual = balances.accrual + EXCLUDED.accrual,
		updated_at = EXCLUDED.updated_at;
`

var (
	ErrQueueIsEmpty = errors.New("queue is empty")
)

//go:generate $GOPATH/bin/mockgen -package=workers -destination=repository_mock.go  -source=$GOFILE
//go:generate $GOPATH/bin/mockgen -package=workers -destination=pg_instance_mock.go -source=../../adapters/pgc/pg_instance.go
//go:generate $GOPATH/bin/mockgen -package=workers  -destination=pgx_mock.go        github.com/jackc/pgx/v5 Tx,Row,BatchResults

type WorkersMetricsRepoIFace interface {
	ObserveDB(op metrics.OpType, duration time.Duration)
	IncAcquireAttempt(found bool)                // фиксируем, были ли задачи в очереди
	ObserveListSize(size int)                    // метрика для мониторинга объема данных
	IncOrderFinalized(status domain.OrderStatus) // метрика финализации
}

type WorkersRepo struct {
	pg      pgc.PgInstance
	metrics WorkersMetricsRepoIFace
	now     func() time.Time
}

func NewWorkersRepo(pg pgc.PgInstance, m WorkersMetricsRepoIFace) *WorkersRepo {
	return &WorkersRepo{
		pg:      pg,
		metrics: m,
		now:     time.Now,
	}
}

// AcquireNextOrder резервирует следующий заказ для обработки воркером
func (r *WorkersRepo) AcquireNextOrder(ctx context.Context) (domain.OrderNumber, domain.OrderStatus, error) {
	start := r.now()
	defer func() {
		if r.metrics != nil {
			r.metrics.ObserveDB(metrics.OpQuery, time.Since(start))
		}
	}()

	var number domain.OrderNumber
	var status domain.OrderStatus

	err := r.pg.PgPool(ctx, func(ctx context.Context, pool pgc.PgxPoolIface) error {
		return pool.QueryRow(ctx, sqlAcquireNextOrder, r.now()).Scan(&number, &status)
	})

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if r.metrics != nil {
				r.metrics.IncAcquireAttempt(false) // Очередь пуста
			}
			return "", "", ErrQueueIsEmpty
		}
		return "", "", fmt.Errorf("acquire order fail: %w", err)
	}

	if r.metrics != nil {
		r.metrics.IncAcquireAttempt(true) // Заказ успешно захвачен
	}
	return number, status, nil
}

// UpdateOrderStatus обновляет статус заказа и сумму начисления.
func (r *WorkersRepo) UpdateOrderStatus(
	ctx context.Context,
	orderNumber domain.OrderNumber,
	orderStatus domain.OrderStatus,
	accrual domain.Amount,
) error {
	start := r.now()
	defer func() {
		if r.metrics != nil {
			r.metrics.ObserveDB(metrics.OpExec, time.Since(start))
		}
	}()

	err := r.pg.PgPool(ctx, func(ctx context.Context, pool pgc.PgxPoolIface) error {
		tag, err := pool.Exec(ctx, sqlUpdateOrderAndBalance, orderNumber, orderStatus, accrual, r.now())
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			slog.Info("order already processed or not found",
				slog.String("order", orderNumber.String()),
			)
			return nil
		}
		// Фиксируем переход в финальный статус
		if orderStatus == "PROCESSED" || orderStatus == "INVALID" {
			if r.metrics != nil {
				r.metrics.IncOrderFinalized(orderStatus)
			}
		}
		return nil
	})

	if err != nil {
		slog.Error("db query failed",
			slog.String("op", "WorkersRepo.UpdateOrderStatus"),
			slog.Any("err", err.Error()),
			slog.String("order", orderNumber.String()),
			slog.String("status", string(orderStatus)),
		)
		return fmt.Errorf("update order status fail: %w", err)
	}

	return nil
}
