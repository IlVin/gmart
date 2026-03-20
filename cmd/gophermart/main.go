package main

import (
	"context"
	"gmart/internal/adapters/metrics"
	"gmart/internal/adapters/pgc"
	"gmart/internal/dto"
	"gmart/internal/model/jserr"
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
				os.Setenv(target, val)
			}
		}
	}
}

func main() {

	// ХАК: тесты не понимают Content-Type: application/problem+json (RFC 9457)
	// Поэтому заменяем Content-Type: application/problem+json на Content-Type: application/json
	huma.NewError = func(status int, message string, errs ...error) huma.StatusError {
		return &jserr.JsError{
			Status: status,
			Title:  message,
		}
	}

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
		hooks.OnStart(func() {
			slog.Info("Starting server",
				slog.String("RunAddress", options.RunAddress),
			)

			// Регистратор для метрик
			metricsReg := prometheus.NewRegistry()
			slog.Info("Prometheus initialized")

			// Инстанс PostgreSQL
			pg, err := pgc.NewPgInstance(
				ctx,
				options.DatabaseURI,
				metrics.NewForPgInstance(metricsReg),
			)
			if err != nil {
				slog.Error("failed postgres initialize",
					slog.Any("err", err),
				)
				return
			}
			defer func() {
				slog.Info("closing database connection")
				pg.Close()
			}()

			// Миграции прерывать нельзя
			if err := pg.RunMigrations(context.Background()); err != nil {
				slog.Error("failed to run migrations",
					slog.Any("err", err),
				)
				return
			}

			// Запуск HTTP сервера
			serve.Serve(ctx, &serve.Input{
				Options:    options,
				Pg:         pg,
				MetricsReg: metricsReg,
			})
		})

		hooks.OnStop(func() {
			slog.Info("HUMA.OnStop")
			stop()
		})
	})

	// Run the CLI. When passed no commands, it starts the server.
	cli.Run()

	//	// Конфигурация
	//	cmdArgs := os.Args[1:]
	//	cfg := config.NewConfig()
	//	cfg, err := cfg.Init(
	//		config.WithEnv(),
	//		config.WithCmdArgs(&cmdArgs),
	//	)
	//	if err != nil {
	//		return fmt.Errorf("failed to initialize config object: %w", err)
	//	}
	//	slog.Info("configuration initialized",
	//		slog.String("version", cfg.Version()),
	//	)

	return nil
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
