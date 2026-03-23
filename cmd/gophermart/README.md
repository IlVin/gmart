# GopherMart Entry Point

Основной исполняемый файл приложения. Отвечает за сборку конфигурации, инициализацию системных адаптеров и управление жизненным циклом сервиса.

## Конфигурация (CLI & ENV)

Приложение поддерживает гибкую настройку. Благодаря механизму **FixEnv**, переменные окружения автоматически маппятся на структуру настроек (добавляется префикс `SERVICE_`).


| Флаг CLI | Переменная ENV | Дефолт | Описание |
| :--- | :--- | :--- | :--- |
| `-p`, `--run-address` | `RUN_ADDRESS` | `:8080` | Сетевой адрес сервера (host:port). |
| `-d`, `--database-uri` | `DATABASE_URI` | `""` | Строка подключения к PostgreSQL. |
| `-r`, `--accrual-system-address` | `ACCRUAL_SYSTEM_ADDRESS` | `""` | URL внешней системы начислений. |
| `-m`, `--run-migrations` | `RUN_MIGRATIONS` | `false` | **Флаг активации миграций БД при старте.** |
| `-k`, `--jwt-secret-key` | `JWT_SECRET_KEY` | `""` | Ключ для подписи JWT (HMAC). |
| `-j`, `--jwt-ttl` | `JWT_TTL` | `15m` | Время жизни токена (например, `24h`). |
| `-t`, `--session-ttl` | `SESSION_TTL` | `100000h` | Время жизни сессии в БД. |
| (нет) | `HTTP_READ_HEADER_TIMEOUT` | `5s` | Защита от Slowloris атак. |
| (нет) | `HTTP_IDLE_TIMEOUT` | `30s` | Таймаут простоя Keep-Alive. |

## Управление миграциями

По умолчанию миграции **отключены** для безопасности в распределенных средах. Чтобы приложение автоматически обновило схему БД при запуске, используйте один из способов:

1. **Флаг**: `./gophermart --run-migrations`
2. **ENV**: `RUN_MIGRATIONS=true ./gophermart`
3. **.env файл**: Добавьте строку `RUN_MIGRATIONS=true`.

```go
// Логика запуска в main.go
if options.RunMigrations {
    slog.Info("Executing database migrations...")
    if err := pg.RunMigrations(context.Background()); err != nil {
        slog.Error("Migration failed", "err", err)
        return
    }
}
```

## Жизненный цикл (Startup Flow)

1. **FixEnv**: Рефлексивный маппинг `dto.CLIOptions` в ENV-переменные для совместимости с `humacli`.
2. **Graceful Shutdown**: Регистрация `NotifyContext` для перехвата `SIGINT/SIGTERM`.
3. **Infrastructure**:
    - Инициализация **Prometheus Registry** (включая Go & Process collectors).
    - Создание инстанса **PostgreSQL** с Circuit Breaker логикой.
4. **Conditional Migrations**: Проверка флага `--run-migrations` и запуск Goose-миграций.
5. **Serve**: Передача управления в `internal/serve` для запуска HTTP-сервера и фоновых воркеров.

## Аргументация дизайна

- **Reflective Mapping**: Позволяет избежать дублирования имен переменных и жесткой привязки к префиксам фреймворка в бизнес-конфигурации.
- **Atomic Migrations**: Использование `context.Background()` для миграций гарантирует, что они не будут прерваны сигналом остановки в процессе выполнения, что предотвращает повреждение стейта БД.
- **Telemetry-First**: Метрики инициализируются до базы данных, что позволяет отслеживать ошибки подключения к PostgreSQL с первой секунды запуска.
