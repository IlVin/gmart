# Module: Postgres Instance (pgc/instance)

`PgInstance` — это интеллектуальная обертка над пулом соединений PostgreSQL (`pgxpool`), реализующая паттерны **Circuit Breaker**, **Retry** и **Self-Healing**. Модуль минимизирует влияние сбоев базы данных на приложение и предотвращает каскадные ошибки.

---

## Основные возможности

*   **Circuit Breaker**: Автоматический переход в `Offline` при накоплении ошибок (3 ошибки за 2 секунды) и блокировка запросов.
*   **Умное восстановление (Probing)**: В состоянии `Offline` только одна горутина раз в 5 секунд (`probeInterval`) получает право проверить доступность БД (`Ping`), предотвращая "шторм запросов".
*   **Безопасные транзакции**: Автоматическое управление `Begin`, `Commit` и `Rollback` с защитой от паник (`recover`) и логированием стека.
*   **Миграции**: Встроенная поддержка `goose` для управления схемой БД при старте.
*   **Метрики**: Сбор данных о задержках (Latency), статусе инстанса и количестве повторных попыток (Retries).

---

## Использование

### Инициализация и запуск
Создание инстанса требует строку подключения и реализацию интерфейса метрик.

```go
ctx := context.Background()
pg, err := instance.NewPgInstance(ctx, "postgres://user:pass@localhost:5432/db", metrics)

// Запуск миграций из встроенной FS
if err := pg.RunMigrations(ctx); err != nil {
    log.Fatal(err)
}
```

### Выполнение транзакций (Tx)
Метод `Tx` инкапсулирует логику транзакции. Если коллбек возвращает ошибку или происходит паника, транзакция откатывается автоматически.

```go
err := pg.Tx(ctx, func(ctx context.Context, tx instance.PgxTxIface) error {
    _, err := tx.Exec(ctx, "INSERT INTO links (url) VALUES ($1)", "https://example.com")
    return err 
})
```

### Прямой доступ к пулу (PgPool)
Для одиночных запросов без транзакции используйте `PgPool`:

```go
err := pg.PgPool(ctx, func(ctx context.Context, pool instance.PgxPoolIface) error {
    return pool.QueryRow(ctx, "SELECT count(*) FROM links").Scan(&count)
})
```

---

## Typed Queries (Prepared Statements)

Для удобной работы с типизированными данными и исключения дублирования логики маппинга используйте хелперы из `prepared_statement.go`. Они базируются на дженериках и новых итераторах Go 1.23.

### Определение запроса (Query & Binder)
Сначала опишите структуру данных и функцию маппинга (`Binder`):

```go
type User struct {
    ID   int
    Name string
}

var GetUser = pgc.NewQuery[User](
    "SELECT id, name FROM users WHERE id = $1",
    func(u *User) []any {
        return []any{&u.ID, &u.Name}
    },
)
```

### Выполнение запросов
#### FetchOne (Одиночная запись)
Автоматически выполняет запрос через `PgPool`, делает `Scan` и возвращает готовую структуру или ошибку (включая `pgx.ErrNoRows`).
```go
user, err := pgc.FetchOne(ctx, GetUser(pg), userID)
```

#### QueryAll (Итератор Go 1.23)
Возвращает итератор `iter.Seq2`, который позволяет стримить данные из БД без аллокации больших слайсов. Безопасно закрывает `rows` после завершения цикла.
```go
for user, err := range pgc.QueryAll(ctx, GetAllUsers(pg)) {
    if err != nil {
        return err
    }
    fmt.Println(user.Name)
}
```

#### Exec (Команды)
Для `INSERT`, `UPDATE` или `DELETE`. Возвращает количество затронутых строк (`RowsAffected`).

```go
rows, err := pgc.Exec(ctx, UpdateName(pg), "NewName", userID)
```



## Технические детали и защита

### Гарантия завершения
В операциях `Commit` и `Rollback` используется `context.WithoutCancel`. Это гарантирует, что транзакция в БД будет корректно закрыта, даже если контекст вызывающего запроса был отменен по таймауту.

### Обработка Panic
Любая паника внутри коллбека перехватывается. Модуль логирует `debug.Stack()`, выполняет `Rollback` (в случае транзакции) и возвращает ошибку, предотвращая падение всего микросервиса.

### Интерфейсы и моки
Модуль разделяет методы пула и транзакции через интерфейсы `PgxPoolIface` и `PgxTxIface`, что упрощает тестирование.

```bash
# Генерация моков для тестирования репозиториев
go generate ./internal/adapters/pgc/instance/pg_instance.go
go generate ./internal/adapters/pgc/prepared_statement.go
```
### Тестирование с типизированными запросами (Gomock)

Поскольку хелперы (`FetchOne`, `QueryAll`) внутри используют `PgPool`, в тестах необходимо имитировать выполнение коллбека.

#### Пример мока для FetchOne:
```go
// 1. Имитируем вход в PgPool и прокидываем mockPool в коллбек
mockPg.EXPECT().
    PgPool(ctx, gomock.Any()).
    DoAndReturn(func(ctx context.Context, cb func(context.Context, pgc.PgxPoolIface) error) error {
        return cb(ctx, mockPool)
    })

// 2. Настраиваем ожидание конкретного SQL и маппинг данных через Scan
mockRow := NewMockRow(ctrl)
mockPool.EXPECT().QueryRow(ctx, sqlGetUser, userID).Return(mockRow)

mockRow.EXPECT().Scan(gomock.Any()).DoAndReturn(func(dest ...any) error {
    *(dest[0].(*int)) = 1
    *(dest[1].(*string)) = "John Doe"
    return nil
})

// 3. Вызываем тестируемый метод репозитория
user, err := repo.GetByID(ctx, userID)
```

---

## Конфигурация
*   **Failures**: 3 ошибки подряд за 2 сек -> Offline.
*   **Retries**: 3 попытки выполнения операции с экспоненциальной задержкой.
*   **probeInterval**: 5 секунд между попытками восстановления из Offline.
*   **Ping Timeout**: 1 секунда.
