package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gmart/internal/domain"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

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

type AccrualResponse struct {
	Order   domain.OrderNumber `json:"order"`
	Status  domain.OrderStatus `json:"status"`
	Accrual domain.Amount      `json:"accrual,omitempty"`
}

// Если в БД не осталось работы, то воркер идет спать на это время
const sleepDuration time.Duration = 1000 * time.Millisecond

type AccrualWrk struct {
	wg                sync.WaitGroup
	sleepUntilMs      atomic.Int64
	cond              *sync.Cond
	repo              WorkerRepoIFace
	metrics           WorkerMetricsIFace
	httpClient        *http.Client
	accrualServiceURL *url.URL
	isShutdown        atomic.Bool
}

func NewAccrualWrk(repo WorkerRepoIFace, m WorkerMetricsIFace, accrualServiceURL *url.URL) *AccrualWrk {
	return &AccrualWrk{
		repo:              repo,
		cond:              sync.NewCond(&sync.Mutex{}),
		metrics:           m,
		accrualServiceURL: accrualServiceURL,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: &http.Transport{MaxIdleConnsPerHost: 100},
		},
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
				a.cond.Broadcast()
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
					if a.sleepUntilMs.Load() <= a.nowMs() {
						a.sleepUntilMs.Store(a.nowMs() + sleepDuration.Milliseconds())
					}
					if !errors.Is(err, ErrQueueIsEmpty) {
						slog.Warn("do work fail", slog.Any("err", err))
					}
				} else if a.sleepUntilMs.Load() <= a.nowMs() {
					a.WakeUp() // Сигналим спящим о том, что работа есть...
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
	a.cond.Broadcast()
	a.wg.Wait()
}

// doWork фоновый воркер
func (a *AccrualWrk) doWork(ctx context.Context) error {

	orderNumber, _, err := a.repo.AcquireNextOrder(ctx)
	if err != nil {
		return err
	}

	// Формируем URL: base/api/orders/{number}
	u := a.accrualServiceURL.JoinPath("api", "orders", orderNumber.String()).String()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		if a.metrics != nil {
			a.metrics.IncProcessed("error")
		}
		return fmt.Errorf("create request fail (%s): %w", orderNumber, err)
	}

	start := time.Now()
	resp, err := a.httpClient.Do(req)
	if err != nil {
		if a.metrics != nil {
			a.metrics.IncProcessed("error")
		}
		return fmt.Errorf("accrual service unreachable (%s): %w", orderNumber, err)
	}
	defer resp.Body.Close()

	if a.metrics != nil {
		a.metrics.ObserveRequest(strconv.Itoa(resp.StatusCode), time.Since(start))
	}

	switch resp.StatusCode {

	case http.StatusOK:
		var res AccrualResponse
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			if a.metrics != nil {
				a.metrics.IncProcessed("error")
			}
			return fmt.Errorf("accrual decode fail (%s): %w", orderNumber, err)
		}
		// Обновляем статус и баллы в БД
		err = a.repo.UpdateOrderStatus(ctx, orderNumber, res.Status, res.Accrual)
		if a.metrics != nil {
			if err == nil {
				a.metrics.IncProcessed("success")
			} else {
				a.metrics.IncProcessed("error")
			}
		}
		return err

	case http.StatusNoContent:
		// заказ не зарегистрирован в системе расчёта
		slog.Warn("order not found in accrual", "order", orderNumber)
		if a.metrics != nil {
			a.metrics.IncProcessed("no_content")
		}
		return nil

	case http.StatusTooManyRequests:
		if a.metrics != nil {
			a.metrics.IncRateLimit()
			a.metrics.IncProcessed("rate_limit")
		}
		retryAfter, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		if retryAfter <= 0 {
			retryAfter = 60
		}
		a.sleepUntilMs.Store(
			a.nowMs() +
				(time.Duration(retryAfter) * time.Second).Milliseconds())
		return fmt.Errorf("rate limited: retry after %d s (%s)", retryAfter, orderNumber)

	case http.StatusInternalServerError:
		if a.metrics != nil {
			a.metrics.IncProcessed("error_500")
		}
		a.sleepUntilMs.Store(
			a.nowMs() +
				(time.Duration(15) * time.Second).Milliseconds())
		return errors.New("internal server error")

	default:
		if a.metrics != nil {
			a.metrics.IncProcessed("error_other")
		}
		return fmt.Errorf("unexpected status code (%s): %d", orderNumber, resp.StatusCode)
	}
}
