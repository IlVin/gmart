-- +goose Up
-- +goose StatementBegin

-- 1. Таблица балансов (агрегат накопительных итогов)
CREATE TABLE IF NOT EXISTS balances (
    user_id     BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    accrual     BIGINT NOT NULL DEFAULT 0,  -- Сумма всех начислений
    withdrawn   BIGINT NOT NULL DEFAULT 0,  -- Сумма всех списаний
    updated_at  TIMESTAMPTZ DEFAULT NOW(),
    
    -- Главная бизнес-проверка: баланс не может быть отрицательным
    CONSTRAINT check_balance_not_negative CHECK (accrual - withdrawn >= 0)
);

-- 2. Таблица списаний
CREATE TABLE IF NOT EXISTS withdrawals (
    order_number  VARCHAR(255) NOT NULL PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount        BIGINT NOT NULL, 
    processed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS withdrawals;
DROP TABLE IF EXISTS balances;

-- +goose StatementEnd
