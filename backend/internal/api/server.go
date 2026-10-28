package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct{ DB *pgxpool.Pool }
type endpoint func(http.ResponseWriter, *http.Request) error

func New(db *pgxpool.Pool) http.Handler {
	s := &Server{DB: db}
	m := http.NewServeMux()
	routes := map[string]endpoint{
		"GET /api/health": s.health, "GET /api/products": s.products, "GET /api/summary": s.summary,
		"POST /api/products": s.createProduct, "PUT /api/products/{id}": s.updateProduct, "DELETE /api/products/{id}": s.deleteProduct,
		"POST /api/products/{id}/stock": s.adjustStock, "GET /api/products/{id}/movements": s.movements,
		"POST /api/orders": s.createOrder, "GET /api/orders/{id}": s.order, "POST /api/orders/{id}/cancel": s.cancelOrder,
		"GET /api/orders": s.orders,
	}
	for route, h := range routes {
		m.HandleFunc(route, wrap(h))
	}
	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, problem(404, "NOT_FOUND", "Endpoint not found"))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		m.ServeHTTP(w, r.WithContext(ctx))
	})
}
func wrap(h endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			writeError(w, err)
		}
	}
}
func writeError(w http.ResponseWriter, err error) {
	var e *APIError
	var pg *pgconn.PgError
	if !errors.As(err, &e) {
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			e = &APIError{Status: 404, Code: "NOT_FOUND", Message: "Resource not found"}
		case errors.As(err, &pg) && pg.Code == "23505":
			e = &APIError{Status: 409, Code: "SKU_EXISTS", Message: "SKU already exists"}
		case errors.As(err, &pg) && pg.Code == "23503":
			e = &APIError{Status: 409, Code: "PRODUCT_HAS_HISTORY", Message: "Product is referenced by history"}
		default:
			slog.Error("request failed", "error", err)
			e = &APIError{Status: 500, Code: "INTERNAL_ERROR", Message: "An unexpected error occurred"}
		}
	}
	_ = respond(w, e.Status, map[string]any{"error": e})
}
func respond(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return problem(400, "VALIDATION_ERROR", "Invalid JSON, unknown field, missing value, or non-integer number")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return problem(400, "VALIDATION_ERROR", "Expected one JSON object")
	}
	return nil
}
func id(r *http.Request) (int64, error) {
	n, err := strconv.ParseInt(r.PathValue("id"), 10, 32)
	if err != nil || n < 1 {
		return 0, problem(400, "VALIDATION_ERROR", "ID must be a positive integer")
	}
	return n, nil
}
func paging(r *http.Request) (int, int, error) {
	page, limit := 1, 20
	var err error
	if v := r.URL.Query().Get("page"); v != "" {
		page, err = strconv.Atoi(v)
		if err != nil || page < 1 || page > 1000000 {
			return 0, 0, problem(400, "VALIDATION_ERROR", "Invalid page")
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err = strconv.Atoi(v)
		if err != nil || limit < 1 || limit > 100 {
			return 0, 0, problem(400, "VALIDATION_ERROR", "Limit must be 1 to 100")
		}
	}
	return page, limit, nil
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) error {
	if err := s.DB.Ping(r.Context()); err != nil {
		return err
	}
	return respond(w, 200, map[string]string{"status": "ok"})
}
