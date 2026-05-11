# RetailCRM — Интеграция: задачи

**Цель:** автоматически синхронизировать каталог товаров из RetailCRM в таблицу `products` (сейчас заполняется вручную через CSV). Дополнительно — возможность отдавать рекомендации как integration module RetailCRM.

---

## Обзор API RetailCRM

- **Auth:** `X-API-KEY` заголовок (не OAuth)
- **Base URL:** `https://{subdomain}.retailcrm.ru/api/v5/`
- **Rate limit:** 10 req/s на IP (503 при превышении)
- **SDK:** `github.com/retailcrm/api-client-go/v2` — официальный Go-клиент с `EnableRateLimiter()`

---

## Таблица маппинга

| Наша таблица `products` | RetailCRM источник | Поле |
|---|---|---|
| `ext_product_id` | `GET /store/products` | `.externalId` |
| `title` | `GET /store/products` | `.name` |
| `price` | `GET /store/offers` | `.price` |
| `url` | `GET /store/products` | `.url` |
| `images` | `GET /store/products` | `.images[]` |
| `tags` | `GET /store/products` | `.properties[]` (кастомные атрибуты) |

---

## Задачи

### 🟥 КРИТИЧНО

#### RETAILCRM-01 — Добавить RetailCRM API key в конфиг

**Что сделать:**
- Добавить в `config.Config`: `RetailCRMURL string`, `RetailCRMAPIKey string`
- Добавить в `config.Load()`: `RETAILCRM_URL`, `RETAILCRM_API_KEY` из env
- Добавить в `.env.template` и `.env.local.template`

**Файлы:** `config/config.go`, `.env.template`, `.env.local.template`

---

#### RETAILCRM-02 — Создать клиент RetailCRM

**Что сделать:**
- Создать пакет `internal/retailcrm/`
- Подключить SDK: `go get github.com/retailcrm/api-client-go/v2`
- Реализовать `Client` с методами:
  - `FetchProducts(ctx, page, limit) ([]Product, error)` — `GET /api/v5/store/products`
  - `FetchOffers(ctx, productIDs []string) (map[string]float64, error)` — `GET /api/v5/store/offers` для цен
- Включить `EnableRateLimiter()` чтобы не получить 503

**Интерфейс:**
```go
type CatalogClient interface {
    FetchProducts(ctx context.Context, page, limit int) ([]Product, error)
}
```

**Файлы:** `internal/retailcrm/client.go`, `internal/retailcrm/types.go`

---

#### RETAILCRM-03 — Синк-воркер каталога

**Что сделать:**
- Создать `internal/retailcrm/syncer.go` — воркер который:
  1. Пагинирует `GET /store/products` (50 штук за раз)
  2. Для каждой страницы: upsert в таблицу `products` по `ext_product_id`
  3. Маппит `.properties[]` → JSONB-теги
  4. Логирует `imported: N, updated: M, errors: K`
- Запускать при старте сервиса + по cron (раз в час или настраиваемо через `RETAILCRM_SYNC_INTERVAL`)

**SQL upsert:**
```sql
INSERT INTO products (ext_product_id, title, price, url, images, tags)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (ext_product_id) DO UPDATE
SET title=EXCLUDED.title, price=EXCLUDED.price,
    url=EXCLUDED.url, images=EXCLUDED.images,
    tags=EXCLUDED.tags, updated_at=CURRENT_TIMESTAMP
```

**Файлы:** `internal/retailcrm/syncer.go`

---

### 🟧 ВАЖНО

#### RETAILCRM-04 — Подключить воркер в main.go

**Что сделать:**
- Инициализировать `retailcrm.NewClient(cfg)` в `cmd/main.go`
- Запустить `syncer.Start(ctx)` в отдельной горутине
- Добавить graceful shutdown для воркера

**Файлы:** `cmd/main.go`

---

#### RETAILCRM-05 — Тесты клиента и синкера

**Что сделать:**
- Unit тест клиента с httptest-мок-сервером RetailCRM ответов
- Integration тест синкера с testcontainers PostgreSQL:
  - Прогнать синк с фейковыми данными
  - Проверить что `products` заполнена корректно
  - Проверить idempotency (повторный запуск не дублирует)

**Файлы:** `internal/retailcrm/client_test.go`, `internal/retailcrm/syncer_test.go`

---

#### RETAILCRM-06 — Admin endpoint для ручного тригера синка

**Что сделать:**
- `POST /api/v1/admin/catalog/sync` — запускает синхронизацию вручную (без ожидания)
- Возвращает `{ "status": "sync_started" }`
- Защищён Admin Bearer-токеном

**Файлы:** `internal/handlers/admin_catalog.go`

---

### 🟨 ЖЕЛАТЕЛЬНО

#### RETAILCRM-07 — История синхронизации

**Что сделать:**
- Таблица `catalog_sync_log`:
  ```sql
  CREATE TABLE catalog_sync_log (
      id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
      started_at TIMESTAMP NOT NULL,
      finished_at TIMESTAMP,
      imported   INTEGER DEFAULT 0,
      updated    INTEGER DEFAULT 0,
      errors     INTEGER DEFAULT 0,
      status     VARCHAR(20) NOT NULL  -- running|completed|failed
  );
  ```
- Заполнять при каждом запуске синкера
- `GET /api/v1/admin/catalog/sync/history` — последние N запусков

---

#### RETAILCRM-08 — Integration Module (рекомендации → RetailCRM)

**Что сделать:**
- Зарегистрировать backend как integration module через `POST /api/v5/integration-modules/{code}/edit`
- RetailCRM будет вызывать наш `GET /api/v1/retailcrm/recommendations?ids[]=...&mode=...`
- Возвращать список `externalId` рекомендованных товаров
- Это позволяет показывать персональные рекомендации прямо в интерфейсе RetailCRM

**Файлы:** `internal/handlers/retailcrm_recommendations.go`

---

## Переменные окружения (добавить)

| Переменная | Описание | Пример |
|-----------|---------|--------|
| `RETAILCRM_URL` | URL вашего RetailCRM | `https://myshop.retailcrm.ru` |
| `RETAILCRM_API_KEY` | API ключ RetailCRM | `abc123...` |
| `RETAILCRM_SYNC_INTERVAL` | Интервал авто-синка | `1h` (default) |

---

## Порядок выполнения

```
RETAILCRM-01 → RETAILCRM-02 → RETAILCRM-03 → RETAILCRM-04
                                    ↓
                              RETAILCRM-05
                                    ↓
                              RETAILCRM-06 (optional)
                              RETAILCRM-07 (optional)
                              RETAILCRM-08 (optional)
```

---

## Что это даёт

- **Сейчас:** каталог заполняется вручную через CSV в Admin Panel
- **После:** каталог синхронизируется автоматически из RetailCRM раз в час
- **Бонус:** все обновления цен, изображений, тегов — подтягиваются без ручного вмешательства
- **Бонус-2:** integration module позволяет показывать рекомендации прямо в RetailCRM
