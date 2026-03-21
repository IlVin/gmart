package serve

import (
	"context"
	"errors"
	"fmt"
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
	humaAPI := InitHuma(mux)

	accrualURL, err := url.Parse(arg.Options.AccrualSystemAddress)
	if err != nil {
		return fmt.Errorf("cannot parse Accrual Sustem Address: %w", err)
	}

	// Токен генератор/валидатор
	tokenGenerator, err := auth.NewTokenGenerator(jwt.SigningMethodHS256, []byte(arg.Options.JwtSecretKey), arg.Options.JwtTtl)
	if err != nil {
		return fmt.Errorf("token generator create fail: %w", err)
	}
	tokenVerifier, err := auth.NewTokenVerifier(jwt.SigningMethodHS256, []byte(arg.Options.JwtSecretKey))
	if err != nil {
		return fmt.Errorf("failed to create token verifier: %w", err)
	}

	// Repo
	authRepo := user.NewAuthRepo(arg.Pg, arg.Options.SessionTtl, metrics.NewForAuth(arg.MetricsReg))
	ordersRepo := orders.NewOrdersRepo(arg.Pg, metrics.NewForOrders(arg.MetricsReg))
	loyaltyRepo := loyalty.NewLoyaltyRepo(arg.Pg, metrics.NewForLoyalty(arg.MetricsReg))

	// Vertical Slices
	user := user.NewUser(authRepo, tokenGenerator)
	orders := orders.NewOrders(ordersRepo)
	loyalty := loyalty.NewLoyalty(loyaltyRepo)
	worker := workers.NewAccrualWrk(ordersRepo, metrics.NewForWorkers(arg.MetricsReg), accrualURL)

	// Регистрация роутингов
	user.RegistryRoutes(humaAPI)
	orders.RegistryRoutes(humaAPI, tokenVerifier)
	loyalty.RegistryRoutes(humaAPI, tokenVerifier)

	// 3. Запускаем воркеры в отдельной горутине
	// Run внутри себя сделает wg.Add и в конце wg.Wait
	worker.Run(ctx, 3)

	// Настройка HTTP сервера
	srv := &http.Server{
		Addr:              arg.Options.RunAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second, // Защита от Slowloris атак
		IdleTimeout:       30 * time.Second,
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

	grp := huma.NewGroup(api)

	grp.UseTransformer(func(ctx huma.Context, status string, v any) (any, error) {
		slog.Warn("Huma Validation Error (422)",
			slog.String("method", ctx.Method()),
			slog.String("path", ctx.URL().Path),
			slog.Any("details", v), // v содержит структуру huma.Error или []huma.ErrorDetail
		)

		// Возвращаем объект без изменений, чтобы Huma отправила его клиенту
		return v, nil
	})

	return grp
}
