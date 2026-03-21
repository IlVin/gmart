package domain

import (
	"time"
)

// WithdrawalItem описывает одну запись в истории списаний
type Withdrawal struct {
	OrderNumber OrderNumber `json:"order"`
	Amount      Amount      `json:"sum"`
	ProcessedAt time.Time   `json:"processed_at"`
}
