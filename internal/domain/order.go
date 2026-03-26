package domain

import (
	"time"
)

type Order struct {
	OrderNumber OrderNumber `json:"number"`
	Status      OrderStatus `json:"status"`
	Amount      Amount      `json:"accrual,omitzero"`
	UploadedAt  time.Time   `json:"uploaded_at"`
}
