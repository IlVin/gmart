package workers

import (
	"context"
	"errors"
	"fmt"
	"gmart/internal/adapters/accrual"
	"gmart/internal/domain"
	"gmart/internal/dto"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

//go:generate $GOPATH/bin/mockgen -package=workers -destination=worker_mock_test.go -source=$GOFILE

type AccrualClientIFace interface {
	Fetch(ctx context.Context, order domain.OrderNumber) (*dto.AccrualResponse, error)
}

type WorkerRepoIFace interface {
	// Взять из БД номер ордера, подлежащего обработке
	AcquireNextOrder(ctx context.Context) (domain.OrderNumber, domain.OrderStatus, error)

	// Обновить статус ордера
	UpdateOrderStatus(ctx context.Context, orderNumber domain.OrderNumber, orderStatus domain.OrderStatus, accrual domain.Amount) error
}

type WorkerMetricsIFace interface {
	// Фиксирует результат итерации воркера (success, error, timeout, no_content)
	IncProcessed(result string)

	// Записывает длительность запроса к Accrual Service с кодом ответа
	ObserveRequest(code string, duration time.Duration)

	// Специфичный счетчик для 429 ошибки, чтобы видеть плотность лимитов на графике
	IncRateLimit()
}

// Если в БД не осталось работы, то воркер идет спать на это время
const sleepDuration time.Duration = 1000 * time.Millisecond
const dbLimiter = 2

type AccrualWrk struct {
	wg            sync.WaitGroup
	sleepUntilMs  atomic.Int64
	cond          *sync.Cond
	repo          WorkerRepoIFace
	metrics       WorkerMetricsIFace
	accrualClient AccrualClientIFace
	isShutdown    atomic.Bool
	dbLimiter     chan struct{}
}

func NewAccrualWrk(repo WorkerRepoIFace, m WorkerMetricsIFace, accrualClient AccrualClientIFace) *AccrualWrk {
	return &AccrualWrk{
		repo:          repo,
		cond:          sync.NewCond(&sync.Mutex{}),
		metrics:       m,
		accrualClient: accrualClient,
		dbLimiter:     make(chan struct{}, dbLimiter),
	}
}

func (a *AccrualWrk) nowMs() int64 {
	return time.Now().UnixMilli()
}

// wrkSleep здесь спят безработные воркеры
func (a *AccrualWrk) wrkSleep(ctx context.Context) {
	a.cond.L.Lock()
	for a.sleepUntilMs.Load() > a.nowMs() && !a.isShutdown.Load() {
		a.cond.Wait() // Воркер спит здесь и не потребляет CPU
	}
	a.cond.L.Unlock()
}

// WakeUp будит один воркер, чтобы тот пошел в БД и взял джобу
func (a *AccrualWrk) WakeUp() {
	a.cond.Signal()
}

func (a *AccrualWrk) incSleepUntilMs(delta int64) {
	newSleepUntilMs, curSleepUntilMs := a.nowMs()+delta, a.sleepUntilMs.Load()
	for newSleepUntilMs > curSleepUntilMs && !a.sleepUntilMs.CompareAndSwap(curSleepUntilMs, newSleepUntilMs) {
		curSleepUntilMs = a.sleepUntilMs.Load()
	}
}

// Run запускает воркеры в работу
func (a *AccrualWrk) Run(ctx context.Context, wrkCount int) {
	// Запускаем тикер
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.WakeUp() // Каждую секунду будим по одному воркеру
			case <-ctx.Done():
				// Тут не можем вызвать Shutdown, т.к. он вызывает a.wg.Wait() (Shutdown нужно вызывать в основном потоке для Graceful Shutdown)
				a.isShutdown.Store(true)
				a.cond.L.Lock()
				a.cond.Broadcast()
				a.cond.L.Unlock()
				return
			}
		}
	}()

	// Запускаем пул воркеров
	for range wrkCount {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			// Жизненный цикл воркера
			for {
				// Делаем работу
				err := a.doWork(ctx)

				// Проверяем завершение работы
				if a.isShutdown.Load() {
					return
				}

				// Если произошла ошибка, то тоже надо поспать
				if err != nil {
					a.incSleepUntilMs(sleepDuration.Milliseconds())
					if !errors.Is(err, ErrQueueIsEmpty) {
						slog.Warn("do work fail", slog.Any("err", err))
					}
				} else if a.sleepUntilMs.Load() <= a.nowMs() {
					continue
				}

				// Спим
				a.wrkSleep(ctx)
			}
		}()
	}
}

func (a *AccrualWrk) Shutdown() {
	a.isShutdown.Store(true)
	a.cond.L.Lock()
	a.cond.Broadcast()
	a.cond.L.Unlock()
	a.wg.Wait()
	close(a.dbLimiter)
}

// doWork фоновый воркер
func (a *AccrualWrk) doWork(ctx context.Context) error {

	// В БД в каждый момент времи ходит не больше dbLimiter воркеров из одного инстанса сервиса
	select {
	case a.dbLimiter <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	orderNumber, _, err := a.repo.AcquireNextOrder(ctx)
	<-a.dbLimiter
	if err != nil {
		return err
	}

	// Сигналим спящим воркерам о том, что в БД есть джобы...
	if a.sleepUntilMs.Load() < a.nowMs() {
		a.WakeUp()
	}

	start := time.Now()

	res, err := a.accrualClient.Fetch(ctx, orderNumber)

	if err != nil {
		if errors.Is(err, accrual.ErrNoContent) {
			// заказ не зарегистрирован в системе расчёта
			slog.Warn("order not found in accrual", "order", orderNumber)
			if a.metrics != nil {
				a.metrics.IncProcessed("no_content")
				a.metrics.ObserveRequest("204", time.Since(start))
			}
			return nil

		} else if errors.Is(err, accrual.ErrTooManyRequests) {
			if a.metrics != nil {
				a.metrics.IncRateLimit()
				a.metrics.IncProcessed("rate_limit")
				a.metrics.ObserveRequest("409", time.Since(start))
			}
			a.incSleepUntilMs(res.RetryAfter.Milliseconds())
			return fmt.Errorf("rate limited: retry after %d ms (%s)", res.RetryAfter.Milliseconds(), orderNumber.String())

		} else if errors.Is(err, accrual.ErrInternalError) {
			if a.metrics != nil {
				a.metrics.IncProcessed("error_500")
				a.metrics.ObserveRequest("500", time.Since(start))
			}
			a.incSleepUntilMs((time.Duration(15) * time.Second).Milliseconds())
			return fmt.Errorf("internal server error (order_number: %s)", orderNumber.String())

		} else {
			if a.metrics != nil {
				a.metrics.IncProcessed("error_other")
				a.metrics.ObserveRequest("XXX", time.Since(start))
			}
			return fmt.Errorf("unexpected status code (order_number: %s): %w", orderNumber.String(), err)
		}
	}

	if a.metrics != nil {
		a.metrics.ObserveRequest("200", time.Since(start))
	}

	// Обновляем статус и баллы в БД
	err = a.repo.UpdateOrderStatus(ctx, orderNumber, res.Status, res.Accrual)
	if err != nil {
		if a.metrics != nil {
			a.metrics.IncProcessed("error")
		}
		return err
	}

	if a.metrics != nil {
		a.metrics.IncProcessed("success")
	}
	return nil

}
