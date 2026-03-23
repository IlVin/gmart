package dto

import (
	"gmart/internal/domain"
	"time"
)

type AccrualResponse struct {
	Order      domain.OrderNumber `json:"order"`
	Status     domain.OrderStatus `json:"status"`
	Accrual    domain.Amount      `json:"accrual,omitempty"`
	RetryAfter time.Duration      `json:"-"`
}
