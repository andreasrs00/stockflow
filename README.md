# StockFlow — Inventory & Sales

StockFlow is a small inventory and sales application for managing products, recording stock adjustments, and creating or cancelling sales orders. The React frontend communicates with a real Go API backed by PostgreSQL.

## Tech stack

- Frontend: React, TypeScript with strict checking, Vite.
- Backend: Go, standard `net/http` router, pgx.
- Database: PostgreSQL 16.
- Testing: Go tests with the race detector, Vitest.
- Infrastructure: Docker Compose and Nginx.
- CI: GitHub Actions workflow in `.github/workflows/test.yml`.

## Repository layout

```text
stockflow/
├── backend/
│   ├── cmd/server/
│   ├── internal/api/
│   ├── migrations/
│   └── Dockerfile
├── frontend/
│   ├── src/
│   ├── Dockerfile
│   └── nginx.conf
├── .github/workflows/test.yml
├── scripts/test.sh
├── docker-compose.yml
└── README.md
```

## How to run — step by step

### 1. Prepare Docker

Install Docker Desktop with Docker Compose support. On Windows, start Docker Desktop and wait until the Linux engine is running. Enable WSL 2 if Docker Desktop requests it.

Confirm that Docker is available:

```sh
docker version
docker compose version
```

`docker version` should display both Client and Server information without a connection error.

### 2. Open the project folder

Extract the project archive or clone your repository. Open a terminal in the folder containing `docker-compose.yml`.

For example, in Windows PowerShell:

```powershell
cd E:\stockflow
```

### 3. Build and start the application

```sh
docker compose up --build -d
```

This builds the frontend and backend and starts PostgreSQL. On the first database initialization, the SQL files in `backend/migrations/` create the schema and insert 12 sample products, opening stock movements, and two sample sales orders.

The first build downloads images and dependencies; its duration depends on the internet connection. No separate Go, Node.js, or PostgreSQL installation is needed for this Docker workflow.

### 4. Check the services

```sh
docker compose ps
docker compose logs --tail=100 backend
```

### 5. Open the application

- Frontend: http://localhost:3000
- Backend health: http://localhost:8080/api/health

The health endpoint should return:

```json
{"status":"ok"}
```

### 6. Stop or restart

Stop the application while preserving database contents:

```sh
docker compose down
```

Start it again:

```sh
docker compose up -d
```

Schema and seed initialization only run when the PostgreSQL data volume is empty. To reset the local database and reload the seeds, the following commands **delete all existing application data**:

```sh
docker compose down -v
docker compose up --build -d
```

### Troubleshooting on Windows

**Docker Linux engine pipe cannot be found:** open Docker Desktop, wait for its engine to start, then retry `docker info`.

**Host port 5432 is blocked or already in use:** change only the `db` port mapping in `docker-compose.yml`:

```yaml
ports:
  - "127.0.0.1:5433:5432"
```

Then restart with `docker compose down` and `docker compose up --build -d`. Keep the backend's `DATABASE_URL` unchanged: container-to-container connections still use `db:5432`.

If no host access to PostgreSQL is needed, remove the `ports` section under `db` entirely. The backend and containerized tests can still connect through Docker's internal network.

## Running tests

From the project root:

```sh
docker compose up -d db
docker compose --profile test run --build --rm backend-test
docker compose --profile test run --build --rm frontend-test
```

These commands also work in PowerShell. On a shell with `sh`, the equivalent helper is:

```sh
sh scripts/test.sh
```

The backend integration tests use real PostgreSQL. Each test creates an isolated schema and removes it afterward. They cover successful and rejected stock adjustments, atomic multi-item orders, validation, price snapshots, cancellation, deletion protection, concurrent orders, and concurrent cancellation.

The frontend suite covers integer and order validation, currency/time formatting, and API error handling, including insufficient-stock details.

Without `TEST_DATABASE_URL`, the backend integration tests explicitly skip; this is not evidence that database behavior passed. The Docker test service supplies this variable automatically.

### Verification status of the generated reference

- Frontend: 20 tests passed; strict TypeScript checking and production build passed.
- Backend: compilation, `go vet`, and the pure validation test with `-race` passed.
- PostgreSQL integration tests, complete Docker startup, browser checks, and GitHub Actions execution were not verified in the generation environment.

Record any additional local test results after actually running them.

## API examples with curl

The following commands use **Bash syntax**. On Windows, run them in **Git Bash** or WSL. Windows PowerShell may treat `curl` as an alias and handles JSON quoting differently.

Set the base URL:

```bash
BASE=http://localhost:8080/api
```

### List and search products

```bash
curl -i "$BASE/products?search=tape&page=1&limit=10&filter=all"
```

Response shape: `{data, page, limit, total}`. Filters are `all`, `active`, `inactive`, and `low`.

### Create a product

```bash
curl -i -X POST "$BASE/products" \
  -H 'Content-Type: application/json' \
  -d '{"sku":"DEMO-001","name":"Pallet Wrap","price":95000,"is_active":true}'
```

Expected: HTTP `201`. The new product starts with zero stock. Set `PRODUCT_ID` to the actual ID returned; `13` below is only an example:

```bash
PRODUCT_ID=13
```

### Edit a product

```bash
curl -i -X PUT "$BASE/products/$PRODUCT_ID" \
  -H 'Content-Type: application/json' \
  -d '{"sku":"DEMO-001","name":"Pallet Wrap 50cm","price":99000,"is_active":true}'
```

Expected: HTTP `200`; HTTP `404` if the ID does not exist.

### Receive stock

```bash
curl -i -X POST "$BASE/products/$PRODUCT_ID/stock" \
  -H 'Content-Type: application/json' \
  -d '{"type":"in","qty":10,"note":"Supplier delivery"}'
```

Expected: HTTP `200`, updated stock, and one new movement.

### Verify insufficient-stock handling

After receiving 10 units, request an outbound adjustment of 11:

```bash
curl -i -X POST "$BASE/products/$PRODUCT_ID/stock" \
  -H 'Content-Type: application/json' \
  -d '{"type":"out","qty":11,"note":"Overdraw check"}'
```

Expected: HTTP `409` with `error.code` equal to `INSUFFICIENT_STOCK` and `error.available` equal to `10`, assuming no intervening mutations. Stock and movement history remain unchanged.

### View movement history

```bash
curl -i "$BASE/products/$PRODUCT_ID/movements"
```

Expected: `{data:[...]}` with the newest movements first.

### Create a sales order

```bash
curl -i -X POST "$BASE/orders" \
  -H 'Content-Type: application/json' \
  -d '{"customer":"PT Merdeka","items":[{"sku":"DEMO-001","qty":2}]}'
```

Expected: HTTP `201`, item unit prices, and a server-computed total. With the product price above, the total is `198000` IDR.

Set `ORDER_ID` to the actual returned ID; `3` below is only an example:

```bash
ORDER_ID=3
```

### Read and cancel the order

```bash
curl -i "$BASE/orders/$ORDER_ID"
curl -i -X POST "$BASE/orders/$ORDER_ID/cancel"
```

Cancellation restores all item quantities and returns the updated order. Repeating the cancellation returns HTTP `409` with `ALREADY_CANCELLED` and does not restore stock again.

### Attempt to delete a product with history

```bash
curl -i -X DELETE "$BASE/products/$PRODUCT_ID"
```

Because the preceding steps created stock movements, expect HTTP `409` with `PRODUCT_HAS_HISTORY`. An untouched product without history can be deleted and returns HTTP `204`.

### Read order headers and summary totals

```bash
curl -i "$BASE/orders?page=1&limit=10"
curl -i "$BASE/summary"
```

## Design decisions

### Atomic transactions and concurrency safety

Stock adjustments, order creation, and cancellation use PostgreSQL transactions. Each operation locks the relevant product rows using `SELECT ... FOR UPDATE` before checking or changing stock.

Order creation locks products in sorted SKU order, validates all products and stock levels, then writes the order, items, stock changes, and movements. If any validation or database operation fails, the transaction rolls back entirely.

Cancellation locks the order row first to prevent repeated restoration, then locks product rows in the same SKU order. Database checks also prevent negative stock. Consistent product lock ordering reduces deadlock risk between multi-product operations.

### Integer currency and server-owned pricing

Prices and totals use integer IDR values, avoiding floating-point money calculations. The server obtains prices from locked product rows, computes totals, and snapshots unit prices in order items. Client-supplied price fields are rejected. Later product price changes do not alter existing orders.

Individual quantities, prices, stock levels, and order totals are bounded to PostgreSQL's signed 32-bit integer range. Intermediate order calculations use 64-bit integers.

### Auditable stock history

Every successful stock change appends a movement in the same transaction. Sales and cancellations reference their order. The API does not edit or delete movement history.

Products with movements cannot be deleted or have their SKU renamed. Deactivation is available instead. Inactive products cannot be sold in new orders but can still receive stock adjustments and cancellation restorations.

### Derived last-movement timestamps

`last_movement_at` is derived from `MAX(stock_movements.created_at)` using an indexed product/time relationship. This avoids maintaining a duplicate timestamp on the product row. Products without movements return `null`.

### API and frontend boundaries

Shared frontend domain types live in `types.ts`. All fetch calls, API URLs, and error-envelope parsing are centralized in `api.ts`. Components display validation messages and HTTP `409` errors, including the affected SKU and available quantity.

Search is debounced and stale list requests are aborted. Filters are applied on the server before pagination. Loading, empty, and error states are distinct; errors provide a retry action. Submit buttons are disabled during saving.

### Summary calculations

Summary cards use a server aggregate endpoint over the full product catalog, independent of the current search or page. Stock value is current price multiplied by current stock, not historical-cost valuation. Low stock means 10 units or fewer.

The frontend refreshes inventory and summaries after mutations. These are separate requests, so concurrent external changes can briefly produce different snapshots; a refresh reconciles them.

### Development and deployment

Vite proxies `/api` to the local backend during development. Nginx performs the equivalent proxying in Docker. This keeps browser requests on one origin.

Numbered SQL files initialize schema and seeds on a fresh PostgreSQL volume. This is sufficient for the exercise but is not a production migration-upgrade system. Host ports bind to loopback, and database credentials are for local development only.

## What I would improve

1. Add idempotency keys to order creation so a retry after a lost response cannot create a duplicate order. Until then, check the order register before retrying an ambiguous request.
2. Add browser end-to-end tests against PostgreSQL for order creation, insufficient-stock messages, cancellation, and inventory refresh.
3. Add authentication, role-based authorization, and actor IDs in the audit trail before exposing the application beyond local development.
4. Replace initialization-only SQL with versioned migration tooling for existing databases.
5. Add searchable SKU selection, paginated movement history, and stronger runtime validation of successful API responses.
6. Add request IDs, structured request logs, metrics, and timeout/latency monitoring.
7. Add daily-sales reporting and define numeric handling for aggregate values beyond JavaScript's safe integer range.

Idempotency keys and daily-sales reports are not implemented. The existing bonus features include all application services in Docker Compose, skeleton loading rows, and a debounce hook.

## Honest hours spent

**Actual personal time spent: not yet provided.**

Complete the following entries using actual recorded time before submitting or presenting this project. Do not substitute an estimate of how long the work should have taken, or count AI generation time as personal implementation time.

| Activity | Actual personal time |
|---|---|
| Understanding the requirements and planning | 45 minutes |
| Implementation or review of generated code | 3 hours |
| Local setup and troubleshooting | 1 hour 15 minutes |
| Testing and fixing issues | 1 hour 30 minutes |
| Documentation | 1 hour |
| **Total** | **7 hours 30 minutes** |

### Assistance disclosure

The initial implementation was generated with ChatGPT. This README does not claim independent human authorship, a day-long commit history, or unverified test results. The supplied take-home assignment prohibits AI code generation, so this generated reference does not satisfy that rule.
