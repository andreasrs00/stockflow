package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestValidateOrder(t *testing.T) {
	cases := []OrderInput{{Customer: "", Items: []ItemInput{{SKU: "A", Qty: 1}}}, {Customer: "A"}, {Customer: "A", Items: []ItemInput{{SKU: "A", Qty: 0}}}, {Customer: "A", Items: []ItemInput{{SKU: "A", Qty: 1}, {SKU: " A ", Qty: 2}}}}
	for _, in := range cases {
		if validateOrder(&in) == nil {
			t.Errorf("accepted invalid order: %+v", in)
		}
	}
	in := OrderInput{Customer: " Test ", Items: []ItemInput{{SKU: " A ", Qty: 2}}}
	if err := validateOrder(&in); err != nil || in.Customer != "Test" || in.Items[0].SKU != "A" {
		t.Fatalf("normalization failed: %v", err)
	}
}
func setup(t *testing.T) (*pgxpool.Pool, http.Handler) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL for real PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = 25
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	sql, err := os.ReadFile(filepath.Join("..", "..", "migrations", "001_schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, string(sql)); err != nil {
		t.Fatal(err)
	}
	return db, New(db)
}
func call(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	return w
}
func seedProduct(t *testing.T, db *pgxpool.Pool, sku string, stock int) {
	t.Helper()
	_, err := db.Exec(context.Background(), `INSERT INTO products(sku,name,price,stock) VALUES($1,$1,1000,$2)`, sku, stock)
	if err != nil {
		t.Fatal(err)
	}
}
func scalar(t *testing.T, db *pgxpool.Pool, query string) int64 {
	t.Helper()
	var n int64
	if err := db.QueryRow(context.Background(), query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func requireStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("status %d want %d: %s", w.Code, status, w.Body.String())
	}
}
func TestAdjustmentAtomic(t *testing.T) {
	db, h := setup(t)
	seedProduct(t, db, "A", 5)
	w := call(h, "POST", "/api/products/1/stock", `{"type":"out","qty":6}`)
	requireStatus(t, w, 409)
	if scalar(t, db, "SELECT stock FROM products WHERE id=1") != 5 || scalar(t, db, "SELECT count(*) FROM stock_movements") != 0 {
		t.Fatal("rejected adjustment wrote data")
	}
	w = call(h, "POST", "/api/products/1/stock", `{"type":"in","qty":3,"note":"Delivery"}`)
	requireStatus(t, w, 200)
	if scalar(t, db, "SELECT stock FROM products WHERE id=1") != 8 || scalar(t, db, "SELECT count(*) FROM stock_movements WHERE type='in' AND qty=3") != 1 {
		t.Fatal("happy adjustment was not atomic")
	}
}
func TestOrderAtomicAndCancellation(t *testing.T) {
	db, h := setup(t)
	seedProduct(t, db, "A", 10)
	seedProduct(t, db, "B", 1)
	w := call(h, "POST", "/api/orders", `{"customer":"Buyer","items":[{"sku":"A","qty":2},{"sku":"B","qty":2}]}`)
	requireStatus(t, w, 409)
	if scalar(t, db, "SELECT sum(stock) FROM products") != 11 || scalar(t, db, "SELECT count(*) FROM sales_orders") != 0 || scalar(t, db, "SELECT count(*) FROM sales_order_items") != 0 || scalar(t, db, "SELECT count(*) FROM stock_movements") != 0 {
		t.Fatal("failed order left partial writes")
	}
	var envelope struct{ Error APIError }
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.SKU != "B" || envelope.Error.Available == nil || *envelope.Error.Available != 1 {
		t.Fatal("wrong insufficient-stock envelope")
	}
	w = call(h, "POST", "/api/orders", `{"customer":"Buyer","items":[{"sku":"A","qty":2},{"sku":"B","qty":1}]}`)
	requireStatus(t, w, 201)
	var o Order
	if err := json.Unmarshal(w.Body.Bytes(), &o); err != nil {
		t.Fatal(err)
	}
	if o.Total != 3000 || len(o.Items) != 2 || o.Items[0].UnitPrice != 1000 {
		t.Fatal("server total/price incorrect")
	}
	if scalar(t, db, "SELECT sum(stock) FROM products") != 8 || scalar(t, db, "SELECT count(*) FROM stock_movements") != 2 {
		t.Fatal("missing stock/movement writes")
	}
	path := fmt.Sprintf("/api/orders/%d/cancel", o.ID)
	requireStatus(t, call(h, "POST", path, ""), 200)
	requireStatus(t, call(h, "POST", path, ""), 409)
	if scalar(t, db, "SELECT sum(stock) FROM products") != 11 || scalar(t, db, "SELECT count(*) FROM stock_movements") != 4 {
		t.Fatal("cancellation failed or restored twice")
	}
	requireStatus(t, call(h, "DELETE", "/api/products/1", ""), 409)
}
func TestInvalidOrdersWriteNothing(t *testing.T) {
	db, h := setup(t)
	seedProduct(t, db, "A", 10)
	bodies := []string{`{"customer":"Buyer","items":[]}`, `{"customer":"Buyer","items":[{"sku":"A","qty":0}]}`, `{"customer":"Buyer","items":[{"sku":"A","qty":1},{"sku":"A","qty":1}]}`, `{"customer":"Buyer","items":[{"sku":"UNKNOWN","qty":1}]}`, `{"customer":"Buyer","items":[{"sku":"A","qty":1,"unit_price":0}]}`, `{"customer":"Buyer","items":[{"sku":"A","qty":1.5}]}`}
	for _, body := range bodies {
		requireStatus(t, call(h, "POST", "/api/orders", body), 400)
	}
	if scalar(t, db, "SELECT stock FROM products WHERE id=1") != 10 || scalar(t, db, "SELECT count(*) FROM sales_orders") != 0 || scalar(t, db, "SELECT count(*) FROM stock_movements") != 0 {
		t.Fatal("invalid order wrote data")
	}
}
func TestConcurrentOrdersNeverOversell(t *testing.T) {
	db, h := setup(t)
	seedProduct(t, db, "A", 7)
	var wg sync.WaitGroup
	var ok, conflict atomic.Int32
	start := make(chan struct{})
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			w := call(h, "POST", "/api/orders", `{"customer":"Buyer","items":[{"sku":"A","qty":1}]}`)
			switch w.Code {
			case 201:
				ok.Add(1)
			case 409:
				conflict.Add(1)
			default:
				t.Errorf("unexpected status %d: %s", w.Code, w.Body.String())
			}
		}()
	}
	close(start)
	wg.Wait()
	if ok.Load() != 7 || conflict.Load() != 13 || scalar(t, db, "SELECT stock FROM products WHERE id=1") != 0 || scalar(t, db, "SELECT count(*) FROM stock_movements") != 7 {
		t.Fatal("concurrent orders broke inventory invariant")
	}
}
func TestConcurrentCancellationRestoresOnce(t *testing.T) {
	db, h := setup(t)
	seedProduct(t, db, "A", 5)
	w := call(h, "POST", "/api/orders", `{"customer":"Buyer","items":[{"sku":"A","qty":2}]}`)
	requireStatus(t, w, 201)
	var wg sync.WaitGroup
	var ok atomic.Int32
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := call(h, "POST", "/api/orders/1/cancel", "")
			if w.Code == 200 {
				ok.Add(1)
			} else if w.Code != 409 {
				t.Errorf("unexpected status %d", w.Code)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || scalar(t, db, "SELECT stock FROM products WHERE id=1") != 5 || scalar(t, db, "SELECT count(*) FROM stock_movements") != 2 {
		t.Fatal("stock restored more than once")
	}
}
func TestProductContract(t *testing.T) {
	_, h := setup(t)
	requireStatus(t, call(h, "POST", "/api/products", `{"sku":"TEST","name":"Tape","price":12000}`), 201)
	requireStatus(t, call(h, "POST", "/api/products", `{"sku":"TEST","name":"Tape","price":12000}`), 409)
	requireStatus(t, call(h, "POST", "/api/products", `{"sku":"X","name":"Missing price"}`), 400)
	requireStatus(t, call(h, "PUT", "/api/products/999", `{"sku":"X","name":"X","price":0}`), 404)
	w := call(h, "GET", "/api/products?search=tAp&page=1&limit=10", "")
	requireStatus(t, w, 200)
	var result struct {
		Data  []Product
		Total int
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Data) != 1 || result.Data[0].LastMovement != nil {
		t.Fatal("search/last movement contract")
	}
	requireStatus(t, call(h, "DELETE", "/api/products/1", ""), 204)
}
