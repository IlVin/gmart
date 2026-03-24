package main

import (
	"context"
	"fmt"
	"gmart/internal/adapters/metrics"
	"gmart/internal/adapters/pgc"
	"gmart/internal/dto"
	"gmart/internal/serve"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"syscall"
	"unicode"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/danielgtaylor/huma/v2/humacli"
	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus"
)

func FixEnv() {
	opts := dto.CLIOptions{}
	t := reflect.TypeOf(opts)
	for i := 0; i < t.NumField(); i++ {
		var fieldName strings.Builder
		runes := []rune(t.Field(i).Name)
		for i := 0; i < len(runes); i++ {
			if i > 0 && unicode.IsUpper(runes[i]) && unicode.IsLower(runes[i-1]) {
				fieldName.WriteRune('_')
			}
			fieldName.WriteRune(unicode.ToUpper(runes[i]))
		}
		if val, ok := os.LookupEnv(fieldName.String()); ok {
			target := "SERVICE_" + fieldName.String()
			if _, exists := os.LookupEnv(target); !exists {
				slog.Info("set env",
					slog.String("src", t.Field(i).Name),
					slog.String("dst", target),
				)
				os.Setenv(target, val)
			}
		}
	}
}

func main() {

	// Загрузка .env
	_ = godotenv.Load()

	// HUMA читает переменные окружения только с префиксом "SERVICE_"
	// И этот префикс захардкожен...
	FixEnv()

	// Запускаем программу
	if err := run(); err != nil {
		slog.Error("server terminated with error",
			slog.Any("err", err),
		)
		os.Exit(1)
	}

}

func run() error {

	// Graceful Shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cli := humacli.New(func(hooks humacli.Hooks, options *dto.CLIOptions) {

		hooks.OnStart(func() { Start(ctx, options) })

		hooks.OnStop(func() {
			slog.Info("HUMA.OnStop")
			stop()
		})
	})

	// Run the CLI. When passed no commands, it starts the server.
	cli.Run()

	return nil
}

// Start запускает основной цикл приложения
func Start(ctx context.Context, options *dto.CLIOptions) {
	slog.Info("Starting server", slog.String("RunAddress", options.RunAddress))

	metricsReg, pg, err := SetupResources(ctx, options)
	if err != nil {
		slog.Error("resource setup failed", slog.Any("err", err))
		return
	}
	defer pg.Close()

	if err := serve.Serve(ctx, &serve.Input{
		Options:    options,
		Pg:         pg,
		MetricsReg: metricsReg,
	}); err != nil {
		slog.Error("serve failed", slog.Any("err", err))
	}
}

// SetupResources инициализирует всё необходимое для работы приложения.
func SetupResources(ctx context.Context, options *dto.CLIOptions) (*prometheus.Registry, pgc.PgInstance, error) {
	metricsReg := prometheus.NewRegistry()

	pg, err := pgc.NewPgInstance(
		ctx,
		options.DatabaseURI,
		metrics.NewForPgInstance(metricsReg),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("failed postgres initialize: %w", err)
	}

	if options.RunMigrations {
		// Миграции на отдельном контексте, чтобы не прервать на полпути
		if err := pg.RunMigrations(context.Background()); err != nil {
			pg.Close()
			return nil, nil, fmt.Errorf("failed to run migrations: %w", err)
		}
	}

	return metricsReg, pg, nil
}

func InitHuma(mux *http.ServeMux, options *dto.CLIOptions) huma.API {
	slog.Info("Init HUMA", slog.Any("options", options))

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

	return humago.New(mux, humaConfig)
}
