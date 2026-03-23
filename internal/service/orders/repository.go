package orders

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gmart/internal/adapters/pgc"
	"gmart/internal/domain"
)

const sqlInsertIntoOrders = `
	WITH ins AS (
		INSERT INTO orders (order_number, user_id, status, uploaded_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (order_number) DO NOTHING
		RETURNING user_id
	)
	SELECT user_id, false AS conflict FROM ins
	UNION ALL
	SELECT user_id, true AS conflict FROM orders WHERE order_number = $1
	LIMIT 1;
`

const sqlSelectOrdersByUserID = `
	SELECT order_number, status, COALESCE(accrual, 0) AS accrual, uploaded_at
	FROM orders
	WHERE user_id = $1
	ORDER BY uploaded_at DESC
`

var (
	ErrOrderConflict        = errors.New("order number already uploaded by another user")
	ErrOrderAlreadyUploaded = errors.New("order number already uploaded")
)

//go:generate $GOPATH/bin/mockgen -source=$GOFILE -destination=repository_mock_test.go  -package=orders

type OrdersMetrics interface {
	ObserveDB(op domain.OpType, duration time.Duration)
	IncOrderUpload(status string) // status: new, conflict, already_uploaded
	ObserveListSize(size int)     // метрика для мониторинга объема данных
}

type OrdersRepo struct {
	pg      pgc.PgInstance
	metrics OrdersMetrics
	now     func() time.Time
}

func NewOrdersRepo(pg pgc.PgInstance, m OrdersMetrics) *OrdersRepo {
	return &OrdersRepo{
		pg:      pg,
		metrics: m,
		now:     time.Now,
	}
}

// Upload загружает новый номер заказа в БД
func (r *OrdersRepo) Upload(ctx context.Context, userID domain.UserID, orderNumber domain.OrderNumber) error {
	start := r.now()
	defer func() {
		if r.metrics != nil {
			r.metrics.ObserveDB(domain.OpExec, time.Since(start))
		}
	}()

	var ownerID domain.UserID
	var isConflict bool

	if err := r.pg.PgPool(ctx, func(ctx context.Context, pool pgc.PgxPoolIface) error {
		return pool.QueryRow(ctx, sqlInsertIntoOrders, orderNumber, userID, domain.StatusNew, r.now()).Scan(&ownerID, &isConflict)
	}); err != nil {
		slog.Error("db query failed",
			slog.String("op", "OrdersRepo.Upload"),
			slog.Any("err", err),
			slog.String("order", orderNumber.String()))
		return fmt.Errorf("upload order fail: %w", err)
	}

	if isConflict {
		if ownerID != userID {
			if r.metrics != nil {
				r.metrics.IncOrderUpload("conflict")
			}
			slog.Warn("order conflict", "requested_by", userID, "actual_owner", ownerID, "order", orderNumber)
			return ErrOrderConflict
		}
		if r.metrics != nil {
			r.metrics.IncOrderUpload("already_uploaded")
		}
		return ErrOrderAlreadyUploaded
	}

	if r.metrics != nil {
		r.metrics.IncOrderUpload("new")
	}
	return nil
}

// List выводит список номеров заказов пользователя
func (r *OrdersRepo) List(ctx context.Context, userID domain.UserID) ([]domain.Order, error) {
	start := r.now()
	defer func() {
		if r.metrics != nil {
			r.metrics.ObserveDB(domain.OpQuery, time.Since(start))
		}
	}()

	var result []domain.Order

	err := r.pg.PgPool(ctx, func(ctx context.Context, pool pgc.PgxPoolIface) error {
		rows, err := pool.Query(ctx, sqlSelectOrdersByUserID, userID)
		if err != nil {
			return err
		}
		defer rows.Close()

		for rows.Next() {
			item := domain.Order{}
			if err := rows.Scan(&item.OrderNumber, &item.Status, &item.Amount, &item.UploadedAt); err != nil {
				return fmt.Errorf("scan order row fail: %w", err)
			}
			result = append(result, item)
		}
		return rows.Err()
	})

	if err != nil {
		slog.Error("db query failed", "op", "OrdersRepo.List", "err", err, "user_id", userID)
		return nil, fmt.Errorf("list orders fail: %w", err)
	}

	if r.metrics != nil {
		r.metrics.ObserveListSize(len(result))
	}

	return result, nil
}
