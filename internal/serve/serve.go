package serve

import (
	"context"
	"errors"
	"fmt"
	"gmart/internal/adapters/accrual"
	"gmart/internal/adapters/metrics"
	"gmart/internal/adapters/pgc"
	"gmart/internal/dto"
	"gmart/internal/model/auth"
	"gmart/internal/service/loyalty"
	"gmart/internal/service/orders"
	"gmart/internal/service/user"
	"gmart/internal/service/workers"
	"net/http"
	"net/url"
	"time"

	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/golang-jwt/jwt/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Input входные параметры для Serve
type Input struct {
	Options    *dto.CLIOptions
	Pg         pgc.PgInstance
	MetricsReg *prometheus.Registry
}

// Запуск сервера. Передаем конфиг и роутер
func Serve(ctx context.Context, arg *Input) error {
	slog.Info("Starting server",
		slog.String(
			"ListenAddr",
			arg.Options.RunAddress,
		),
		slog.String(
			"AccrualSystemAddr",
			arg.Options.AccrualSystemAddress,
		),
		slog.String(
			"PgInstance",
			arg.Options.DatabaseURI,
		),
	)

	mux := http.NewServeMux()

	// Регистрируем эндпоинт для сбора метрик (Pull модель)
	// Prometheus будет заходить сюда по адресу /metrics
	arg.MetricsReg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	arg.MetricsReg.MustRegister(collectors.NewGoCollector())
	mux.Handle("/metrics", promhttp.HandlerFor(arg.MetricsReg, promhttp.HandlerOpts{}))

	humaAPI := InitHuma(mux)

	accrualURL, err := url.Parse(arg.Options.AccrualSystemAddress)
	if err != nil {
		return fmt.Errorf("cannot parse Accrual Sustem Address: %w", err)
	}

	// Токен генератор/валидатор
	tokenGenerator, err := auth.NewTokenGenerator(jwt.SigningMethodHS256, []byte(arg.Options.JwtSecretKey), arg.Options.JwtTTL)
	if err != nil {
		return fmt.Errorf("token generator create fail: %w", err)
	}
	tokenVerifier, err := auth.NewTokenVerifier(jwt.SigningMethodHS256, []byte(arg.Options.JwtSecretKey))
	if err != nil {
		return fmt.Errorf("failed to create token verifier: %w", err)
	}

	// Metrics
	mAuth := metrics.NewForAuth(arg.MetricsReg)
	mOrders := metrics.NewForOrders(arg.MetricsReg)
	mLoyalty := metrics.NewForLoyalty(arg.MetricsReg)
	mWorkers := metrics.NewForWorkers(arg.MetricsReg)

	// Repo
	authRepo := user.NewAuthRepo(arg.Pg, arg.Options.SessionTTL, mAuth)
	ordersRepo := orders.NewOrdersRepo(arg.Pg, mOrders)
	loyaltyRepo := loyalty.NewLoyaltyRepo(arg.Pg, mLoyalty)
	workersRepo := workers.NewWorkersRepo(arg.Pg, mOrders)
	accrualClient := accrual.NewClient(accrualURL)

	// Vertical Slices
	user := user.NewUser(authRepo, tokenGenerator)
	orders := orders.NewOrders(ordersRepo)
	loyalty := loyalty.NewLoyalty(loyaltyRepo)
	worker := workers.NewAccrualWrk(workersRepo, mWorkers, accrualClient)

	// Регистрация роутингов
	user.RegistryRoutes(humaAPI)
	orders.RegistryRoutes(humaAPI, tokenVerifier)
	loyalty.RegistryRoutes(humaAPI, tokenVerifier)

	// Запускаем воркеры, если их количество больше 0
	if arg.Options.AccrualWorkers > 0 {
		go func() {
			slog.Info("Starting background workers", slog.Int("count", arg.Options.AccrualWorkers))
			worker.Run(ctx, arg.Options.AccrualWorkers)
		}()
	}

	// Настройка HTTP сервера
	srv := &http.Server{
		Addr:              arg.Options.RunAddress,
		Handler:           mux,
		ReadHeaderTimeout: arg.Options.HttpReadHeaderTimeout,
		IdleTimeout:       arg.Options.HttpIdleTimeout,
	}

	// Канал для ошибок сервера
	serverErrors := make(chan error, 1)

	// Запускаем сервер в горутине
	go func() {
		slog.Info("Server is ready to handle requests")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- fmt.Errorf("listen and serve: %w", err)
			close(serverErrors)
		}
	}()

	// Блокируемся и ждем либо ошибку, либо сигнал отмены контекста
	select {
	case err := <-serverErrors:
		return err

	case <-ctx.Done():
		slog.Info("Shutting down server...")

		// Даем серверу 5 секунд на завершение текущих запросов
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("forced shutdown: %w", err)
		}
		slog.Info("Server stopped gracefully")

		worker.Shutdown()
	}

	return nil
}

// InitHuma возвращает инициализированный HUMA
func InitHuma(mux *http.ServeMux) huma.API {
	slog.Info("Init huma router")

	// API with huma
	humaConfig := huma.DefaultConfig("GopherMart API", "1.0.0")

	humaConfig.Formats["text/plain"] = huma.DefaultFormats["text/plain"]

	humaConfig.Components = &huma.Components{
		SecuritySchemes: map[string]*huma.SecurityScheme{
			"bearer": {
				Type:         "http",
				Scheme:       "bearer",
				BearerFormat: "JWT",
				Description:  "Введите JWT токен для доступа к защищенным ресурсам.",
			},
		},
	}

	api := humago.New(mux, humaConfig)

	return api
}
