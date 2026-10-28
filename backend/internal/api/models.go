package api

import "time"

const maxInt = 2147483647

type Product struct {
	ID           int64      `json:"id"`
	SKU          string     `json:"sku"`
	Name         string     `json:"name"`
	Price        int64      `json:"price"`
	Stock        int64      `json:"stock"`
	Active       bool       `json:"is_active"`
	CreatedAt    time.Time  `json:"created_at"`
	LastMovement *time.Time `json:"last_movement_at"`
}
type ProductInput struct {
	SKU    string `json:"sku"`
	Name   string `json:"name"`
	Price  *int64 `json:"price"`
	Active *bool  `json:"is_active"`
}
type Movement struct {
	ID        int64     `json:"id"`
	ProductID int64     `json:"product_id"`
	Type      string    `json:"type"`
	Qty       int64     `json:"qty"`
	Note      string    `json:"note"`
	OrderID   *int64    `json:"order_id"`
	CreatedAt time.Time `json:"created_at"`
}
type StockInput struct {
	Type string `json:"type"`
	Qty  int64  `json:"qty"`
	Note string `json:"note"`
}
type Item struct {
	SKU       string `json:"sku"`
	Qty       int64  `json:"qty"`
	UnitPrice int64  `json:"unit_price"`
}
type ItemInput struct {
	SKU string `json:"sku"`
	Qty int64  `json:"qty"`
}
type OrderInput struct {
	Customer string      `json:"customer"`
	Items    []ItemInput `json:"items"`
}
type Order struct {
	ID          int64      `json:"id"`
	Customer    string     `json:"customer"`
	Status      string     `json:"status"`
	Total       int64      `json:"total"`
	CreatedAt   time.Time  `json:"created_at"`
	CancelledAt *time.Time `json:"cancelled_at"`
	Items       []Item     `json:"items"`
}
type APIError struct {
	Status    int    `json:"-"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	SKU       string `json:"sku,omitempty"`
	Available *int64 `json:"available,omitempty"`
}

func (e *APIError) Error() string { return e.Message }
func problem(status int, code, message string) error {
	return &APIError{Status: status, Code: code, Message: message}
}
func insufficient(sku string, available int64) error {
	return &APIError{Status: 409, Code: "INSUFFICIENT_STOCK", Message: "Not enough stock for " + sku, SKU: sku, Available: &available}
}
