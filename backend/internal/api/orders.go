package api

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"sort"
	"strings"
)

func validateOrder(in *OrderInput) error {
	in.Customer = strings.TrimSpace(in.Customer)
	if in.Customer == "" || len(in.Customer) > 200 {
		return problem(400, "VALIDATION_ERROR", "Customer is required (max 200 chars)")
	}
	if len(in.Items) == 0 || len(in.Items) > 100 {
		return problem(400, "VALIDATION_ERROR", "An order needs 1 to 100 items")
	}
	seen := map[string]bool{}
	for i := range in.Items {
		item := &in.Items[i]
		item.SKU = strings.TrimSpace(item.SKU)
		if item.SKU == "" || len(item.SKU) > 80 || item.Qty <= 0 || item.Qty > maxInt {
			return problem(400, "VALIDATION_ERROR", "Each item needs a SKU and a positive integer quantity up to 2147483647")
		}
		if seen[item.SKU] {
			return problem(400, "VALIDATION_ERROR", "Duplicate SKU: "+item.SKU)
		}
		seen[item.SKU] = true
	}
	return nil
}
func (s *Server) createOrder(w http.ResponseWriter, r *http.Request) error {
	var in OrderInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if err := validateOrder(&in); err != nil {
		return err
	}
	// Every multi-product transaction locks in the same SKU order to avoid cycles.
	sort.Slice(in.Items, func(i, j int) bool { return in.Items[i].SKU < in.Items[j].SKU })
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	products := make([]Product, len(in.Items))
	var total int64
	for i, item := range in.Items {
		p := &products[i]
		err = tx.QueryRow(r.Context(), `SELECT id,sku,price,stock,is_active FROM products WHERE sku=$1 FOR UPDATE`, item.SKU).Scan(&p.ID, &p.SKU, &p.Price, &p.Stock, &p.Active)
		if errors.Is(err, pgx.ErrNoRows) {
			return problem(400, "VALIDATION_ERROR", "Unknown SKU: "+item.SKU)
		}
		if err != nil {
			return err
		}
		if !p.Active {
			return problem(400, "VALIDATION_ERROR", "Inactive SKU: "+item.SKU)
		}
		subtotal := p.Price * item.Qty
		if subtotal > maxInt-total {
			return problem(400, "VALIDATION_ERROR", "Order total exceeds 2147483647 IDR")
		}
		total += subtotal
	}
	// No write occurs until every item, price, quantity and stock level is validated.
	for i, item := range in.Items {
		if products[i].Stock < item.Qty {
			return insufficient(item.SKU, products[i].Stock)
		}
	}
	var orderID int64
	if err = tx.QueryRow(r.Context(), `INSERT INTO sales_orders(customer,total) VALUES($1,$2) RETURNING id`, in.Customer, total).Scan(&orderID); err != nil {
		return err
	}
	for i, item := range in.Items {
		p := products[i]
		if _, err = tx.Exec(r.Context(), `UPDATE products SET stock=stock-$2 WHERE id=$1`, p.ID, item.Qty); err != nil {
			return err
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO sales_order_items(order_id,sku,qty,unit_price) VALUES($1,$2,$3,$4)`, orderID, item.SKU, item.Qty, p.Price); err != nil {
			return err
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO stock_movements(product_id,type,qty,note,order_id) VALUES($1,'out',$2,'Sales order',$3)`, p.ID, item.Qty, orderID); err != nil {
			return err
		}
	}
	o, err := loadOrder(r.Context(), tx, orderID)
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, 201, o)
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadOrder(ctx context.Context, q queryer, n int64) (Order, error) {
	var o Order
	o.Items = []Item{}
	if err := q.QueryRow(ctx, `SELECT id,customer,status,total,created_at,cancelled_at FROM sales_orders WHERE id=$1`, n).Scan(&o.ID, &o.Customer, &o.Status, &o.Total, &o.CreatedAt, &o.CancelledAt); err != nil {
		return o, err
	}
	rows, err := q.Query(ctx, `SELECT sku,qty,unit_price FROM sales_order_items WHERE order_id=$1 ORDER BY sku`, n)
	if err != nil {
		return o, err
	}
	defer rows.Close()
	for rows.Next() {
		var item Item
		if err = rows.Scan(&item.SKU, &item.Qty, &item.UnitPrice); err != nil {
			return o, err
		}
		o.Items = append(o.Items, item)
	}
	return o, rows.Err()
}
func (s *Server) order(w http.ResponseWriter, r *http.Request) error {
	n, err := id(r)
	if err != nil {
		return err
	}
	o, err := loadOrder(r.Context(), s.DB, n)
	if err != nil {
		return err
	}
	return respond(w, 200, o)
}
func (s *Server) orders(w http.ResponseWriter, r *http.Request) error {
	page, limit, err := paging(r)
	if err != nil {
		return err
	}
	rows, err := s.DB.Query(r.Context(), `SELECT id,customer,status,total,created_at,cancelled_at FROM sales_orders ORDER BY id DESC LIMIT $1 OFFSET $2`, limit, (page-1)*limit)
	if err != nil {
		return err
	}
	defer rows.Close()
	data := []Order{}
	for rows.Next() {
		var o Order
		o.Items = []Item{}
		if err = rows.Scan(&o.ID, &o.Customer, &o.Status, &o.Total, &o.CreatedAt, &o.CancelledAt); err != nil {
			return err
		}
		data = append(data, o)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return respond(w, 200, map[string]any{"data": data, "page": page, "limit": limit})
}
func (s *Server) cancelOrder(w http.ResponseWriter, r *http.Request) error {
	n, err := id(r)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var status string
	if err = tx.QueryRow(r.Context(), `SELECT status FROM sales_orders WHERE id=$1 FOR UPDATE`, n).Scan(&status); err != nil {
		return err
	}
	if status == "CANCELLED" {
		return problem(409, "ALREADY_CANCELLED", "This order has already been cancelled")
	}
	o, err := loadOrder(r.Context(), tx, n)
	if err != nil {
		return err
	}
	// loadOrder returns items in ascending SKU order, matching order creation.
	for _, item := range o.Items {
		var pid, stock int64
		if err = tx.QueryRow(r.Context(), `SELECT id,stock FROM products WHERE sku=$1 FOR UPDATE`, item.SKU).Scan(&pid, &stock); err != nil {
			return err
		}
		if stock > maxInt-item.Qty {
			return problem(409, "STOCK_LIMIT", "Restoring "+item.SKU+" would exceed the stock limit")
		}
		if _, err = tx.Exec(r.Context(), `UPDATE products SET stock=stock+$2 WHERE id=$1`, pid, item.Qty); err != nil {
			return err
		}
		if _, err = tx.Exec(r.Context(), `INSERT INTO stock_movements(product_id,type,qty,note,order_id) VALUES($1,'in',$2,'Order cancellation',$3)`, pid, item.Qty, n); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(r.Context(), `UPDATE sales_orders SET status='CANCELLED',cancelled_at=now() WHERE id=$1`, n); err != nil {
		return err
	}
	o, err = loadOrder(r.Context(), tx, n)
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, 200, o)
}
