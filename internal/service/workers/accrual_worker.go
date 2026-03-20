package workers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gmart/internal/domain"
	"gmart/internal/service/orders"
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

type WorkerMetrics interface {
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
const sleepDuration time.Duration = 1 * time.Second

type AccrualWrk struct {
	wg                sync.WaitGroup
	chWakeUp          chan struct{} // сигнальный канал для побудки
	sleepUntil        atomic.Uint64
	repo              WorkerRepoIFace
	metrics           WorkerMetrics
	httpClient        *http.Client
	accrualServiceURL *url.URL
}

func NewAccrualWrk(repo WorkerRepoIFace, m WorkerMetrics, wakeUpChan chan struct{}, accrualServiceURL *url.URL) *AccrualWrk {
	return &AccrualWrk{
		chWakeUp:          wakeUpChan,
		repo:              repo,
		metrics:           m,
		accrualServiceURL: accrualServiceURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// wrkSleep здесь спят безработные воркеры
func (a *AccrualWrk) wrkSleep(ctx context.Context) error {
	until := time.Unix(int64(a.sleepUntil.Load()), 0)
	remaining := time.Until(until)

	delay := sleepDuration
	if remaining > delay {
		delay = remaining
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	// Спим пока не придет сигнал
	select {
	case _, ok := <-a.chWakeUp:
		if ok {
			return nil
		}
		return errors.New("wake up chan is closed")
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WakeUp будит один воркер, чтобы тот пошел в БД и взял джобу
func (a *AccrualWrk) WakeUp() {
	select {
	case a.chWakeUp <- struct{}{}:
	default:
	}
}

// Run запускает воркеры в работу
func (a *AccrualWrk) Run(ctx context.Context, wrkCount int) {

	// Запускаем пул воркеров
	for range wrkCount {
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			// Жизненный цикл воркера
			for {
				// Делаем работу
				err := a.doWork(ctx)
				if err == nil {
					continue
				}
				if !errors.Is(err, orders.ErrQueueIsEmpty) {
					slog.Warn("do work fail", slog.Any("err", err))
				}

				// Спим
				for {
					err := a.wrkSleep(ctx)
					if err != nil {
						if !errors.Is(err, context.Canceled) {
							slog.Warn("sleep cancelled", slog.Any("err", err))
						}
						return
					}
					if a.sleepUntil.Load() < uint64(time.Now().Unix()) {
						break
					}
				}
			}
		}()
	}
}

func (a *AccrualWrk) Shutdown() {
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
		a.sleepUntil.Store(uint64(time.Now().Add(time.Duration(retryAfter) * time.Second).Unix()))
		return fmt.Errorf("rate limited: retry after %d seconds (%s)", retryAfter, orderNumber)

	case http.StatusInternalServerError:
		if a.metrics != nil {
			a.metrics.IncProcessed("error_500")
		}
		a.sleepUntil.Store(uint64(time.Now().Add(15 * time.Second).Unix()))
		return errors.New("internal server error")

	default:
		if a.metrics != nil {
			a.metrics.IncProcessed("error_other")
		}
		return fmt.Errorf("unexpected status code (%s): %d", orderNumber, resp.StatusCode)
	}
}
