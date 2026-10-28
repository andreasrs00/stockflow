package api

import (
	"github.com/jackc/pgx/v5"
	"net/http"
	"strings"
)

const productFields = `p.id,p.sku,p.name,p.price,p.stock,p.is_active,p.created_at,(SELECT max(m.created_at) FROM stock_movements m WHERE m.product_id=p.id)`

func scanProduct(row pgx.Row) (Product, error) {
	var p Product
	err := row.Scan(&p.ID, &p.SKU, &p.Name, &p.Price, &p.Stock, &p.Active, &p.CreatedAt, &p.LastMovement)
	return p, err
}
func validateProduct(p *ProductInput) error {
	p.SKU = strings.TrimSpace(p.SKU)
	p.Name = strings.TrimSpace(p.Name)
	if p.SKU == "" || len(p.SKU) > 80 || p.Name == "" || len(p.Name) > 200 || p.Price == nil || *p.Price < 0 || *p.Price > maxInt {
		return problem(400, "VALIDATION_ERROR", "SKU (1–80 chars), name (1–200 chars), and integer price (0–2147483647) are required")
	}
	return nil
}
func (s *Server) products(w http.ResponseWriter, r *http.Request) error {
	page, limit, err := paging(r)
	if err != nil {
		return err
	}
	filter := r.URL.Query().Get("filter")
	if filter != "" && filter != "all" && filter != "active" && filter != "inactive" && filter != "low" {
		return problem(400, "VALIDATION_ERROR", "Unknown filter")
	}
	search := strings.TrimSpace(r.URL.Query().Get("search"))
	where := ` WHERE (strpos(lower(p.sku),lower($1))>0 OR strpos(lower(p.name),lower($1))>0) AND ($2 IN ('','all') OR ($2='active' AND p.is_active) OR ($2='inactive' AND NOT p.is_active) OR ($2='low' AND p.stock<=10))`
	tx, err := s.DB.BeginTx(r.Context(), pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var total int64
	if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM products p`+where, search, filter).Scan(&total); err != nil {
		return err
	}
	rows, err := tx.Query(r.Context(), `SELECT `+productFields+` FROM products p`+where+` ORDER BY p.id DESC LIMIT $3 OFFSET $4`, search, filter, limit, (page-1)*limit)
	if err != nil {
		return err
	}
	data := []Product{}
	for rows.Next() {
		p, e := scanProduct(rows)
		if e != nil {
			rows.Close()
			return e
		}
		data = append(data, p)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, 200, map[string]any{"data": data, "page": page, "limit": limit, "total": total})
}
func (s *Server) summary(w http.ResponseWriter, r *http.Request) error {
	var total, active, value, low int64
	err := s.DB.QueryRow(r.Context(), `SELECT count(*),count(*) FILTER(WHERE is_active),COALESCE(sum(price::bigint*stock),0)::bigint,count(*) FILTER(WHERE stock<=10) FROM products`).Scan(&total, &active, &value, &low)
	if err != nil {
		return err
	}
	return respond(w, 200, map[string]int64{"total_products": total, "active_products": active, "total_stock_value": value, "low_stock_count": low})
}
func (s *Server) createProduct(w http.ResponseWriter, r *http.Request) error {
	var in ProductInput
	if err := decode(w, r, &in); err != nil {
		return err
	}
	if err := validateProduct(&in); err != nil {
		return err
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	p, err := scanProduct(s.DB.QueryRow(r.Context(), `INSERT INTO products AS p(sku,name,price,is_active) VALUES($1,$2,$3,$4) RETURNING p.id,p.sku,p.name,p.price,p.stock,p.is_active,p.created_at,NULL::timestamptz`, in.SKU, in.Name, *in.Price, active))
	if err != nil {
		return err
	}
	return respond(w, 201, p)
}
func (s *Server) updateProduct(w http.ResponseWriter, r *http.Request) error {
	n, err := id(r)
	if err != nil {
		return err
	}
	var in ProductInput
	if err = decode(w, r, &in); err != nil {
		return err
	}
	if err = validateProduct(&in); err != nil {
		return err
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var oldSKU string
	var active bool
	if err = tx.QueryRow(r.Context(), `SELECT sku,is_active FROM products WHERE id=$1 FOR UPDATE`, n).Scan(&oldSKU, &active); err != nil {
		return err
	}
	if oldSKU != in.SKU {
		var history bool
		if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM stock_movements WHERE product_id=$1)`, n).Scan(&history); err != nil {
			return err
		}
		if history {
			return problem(409, "PRODUCT_HAS_HISTORY", "SKU cannot change after stock history exists")
		}
	}
	if in.Active != nil {
		active = *in.Active
	}
	if _, err = tx.Exec(r.Context(), `UPDATE products SET sku=$2,name=$3,price=$4,is_active=$5 WHERE id=$1`, n, in.SKU, in.Name, *in.Price, active); err != nil {
		return err
	}
	p, err := scanProduct(tx.QueryRow(r.Context(), `SELECT `+productFields+` FROM products p WHERE p.id=$1`, n))
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, 200, p)
}
func (s *Server) deleteProduct(w http.ResponseWriter, r *http.Request) error {
	n, err := id(r)
	if err != nil {
		return err
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var found int64
	if err = tx.QueryRow(r.Context(), `SELECT id FROM products WHERE id=$1 FOR UPDATE`, n).Scan(&found); err != nil {
		return err
	}
	var history bool
	if err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM stock_movements WHERE product_id=$1)`, n).Scan(&history); err != nil {
		return err
	}
	if history {
		return problem(409, "PRODUCT_HAS_HISTORY", "Products with movement history cannot be deleted; deactivate instead")
	}
	if _, err = tx.Exec(r.Context(), `DELETE FROM products WHERE id=$1`, n); err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	w.WriteHeader(204)
	return nil
}
func (s *Server) movements(w http.ResponseWriter, r *http.Request) error {
	n, err := id(r)
	if err != nil {
		return err
	}
	var found int64
	if err = s.DB.QueryRow(r.Context(), `SELECT id FROM products WHERE id=$1`, n).Scan(&found); err != nil {
		return err
	}
	rows, err := s.DB.Query(r.Context(), `SELECT id,product_id,type,qty,COALESCE(note,''),order_id,created_at FROM stock_movements WHERE product_id=$1 ORDER BY created_at DESC,id DESC`, n)
	if err != nil {
		return err
	}
	defer rows.Close()
	data := []Movement{}
	for rows.Next() {
		var m Movement
		if err = rows.Scan(&m.ID, &m.ProductID, &m.Type, &m.Qty, &m.Note, &m.OrderID, &m.CreatedAt); err != nil {
			return err
		}
		data = append(data, m)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return respond(w, 200, map[string]any{"data": data})
}
func (s *Server) adjustStock(w http.ResponseWriter, r *http.Request) error {
	n, err := id(r)
	if err != nil {
		return err
	}
	var in StockInput
	if err = decode(w, r, &in); err != nil {
		return err
	}
	if (in.Type != "in" && in.Type != "out") || in.Qty <= 0 || in.Qty > maxInt || len(in.Note) > 1000 {
		return problem(400, "VALIDATION_ERROR", "Direction must be in/out, qty a positive integer up to 2147483647, note at most 1000 chars")
	}
	tx, err := s.DB.Begin(r.Context())
	if err != nil {
		return err
	}
	defer tx.Rollback(r.Context())
	var stock int64
	var sku string
	if err = tx.QueryRow(r.Context(), `SELECT stock,sku FROM products WHERE id=$1 FOR UPDATE`, n).Scan(&stock, &sku); err != nil {
		return err
	}
	delta := in.Qty
	if in.Type == "out" {
		if stock < in.Qty {
			return insufficient(sku, stock)
		}
		delta = -in.Qty
	} else if stock > maxInt-in.Qty {
		return problem(409, "STOCK_LIMIT", "Stock would exceed integer limit")
	}
	if _, err = tx.Exec(r.Context(), `UPDATE products SET stock=stock+$2 WHERE id=$1`, n, delta); err != nil {
		return err
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO stock_movements(product_id,type,qty,note) VALUES($1,$2,$3,$4)`, n, in.Type, in.Qty, in.Note); err != nil {
		return err
	}
	p, err := scanProduct(tx.QueryRow(r.Context(), `SELECT `+productFields+` FROM products p WHERE p.id=$1`, n))
	if err != nil {
		return err
	}
	if err = tx.Commit(r.Context()); err != nil {
		return err
	}
	return respond(w, 200, p)
}
