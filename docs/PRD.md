# AI-Астростилист — PRD (Backend API)

**Версия:** 2.0  
**Дата:** 2026-05-11  
**Статус:** In Development  
**Команда:** Backend — Astrostilist

---

## 1. Обзор проекта

**AI-Астростилист** — backend-сервис (middleware), который связывает Telegram-бот заказчика с AI-движком и базой товаров. Пользователь вводит дату рождения → сервис рассчитывает астрологический профиль → выбирает подходящие товары по правилам маппинга → генерирует персональный текст через AI.

### 1.1 Бизнес-задачи

| # | Задача | Метрика успеха |
|---|--------|----------------|
| 1 | Сбор first-party данных (дата рождения) | +X% зарегистрированных профилей |
| 2 | Персонализированные рекомендации товаров | CTR рекомендаций > baseline |
| 3 | Рост повторных покупок через сценарий «День рождения» | +X% repeat purchase rate |

### 1.2 Что входит / не входит в scope

**Входит:**
- Backend REST API для Telegram-бота
- Асинхронный движок обработки через NATS JetStream
- Rule-Engine маппинга астро-параметров на теги товаров
- Синхронизация каталога товаров из RetailCRM
- Web Admin Panel (управление каталогом и правилами)

**НЕ входит:**
- Telegram-бот и пользовательский UI (на стороне заказчика)
- Платёжный функционал и e-commerce витрина
- Нативная интеграция с CRM — синхронизация каталога через API RetailCRM (отдельный модуль)

---

## 2. Архитектура системы

### 2.1 Общая схема

```
┌─────────────────────────────────────────────────────────────────────┐
│                        ВНЕШНИЕ СИСТЕМЫ                               │
│                                                                     │
│  [Telegram Bot]          [RetailCRM]           [YandexGPT / AI]    │
│  (заказчик)              (каталог товаров)      (генерация текста)  │
└──────┬────────────────────────┬─────────────────────┬──────────────┘
       │                        │                     │
       │ HTTP REST              │ API polling          │ HTTP API
       ▼                        ▼                     │
┌─────────────────────────────────────────────────────│──────────────┐
│                     BACKEND (наш сервис)             │              │
│                                                      │              │
│  ┌──────────────────────────────────┐                │              │
│  │         HTTP Router (chi)        │                │              │
│  │                                  │                │              │
│  │  POST /astro/profile             │                │              │
│  │  POST /astro/recommend           │                │              │
│  │  POST /feedback                  │                │              │
│  │  GET  /admin/products            │                │              │
│  │  GET  /admin/logs                │                │              │
│  │  CRUD /admin/rules               │                │              │
│  └────────────┬─────────────────────┘                │              │
│               │ publish                              │              │
│               ▼                                      │              │
│  ┌────────────────────────┐   ┌──────────────────────┘              │
│  │  NATS JetStream        │   │  RetailCRM Sync Worker              │
│  │  astro_events stream   │   │  (polling каталога)                 │
│  │                        │   └──────────────────────┐              │
│  │  [profile worker]      │                          │              │
│  │  [recommend worker]    │◄─ consume                │              │
│  └────────────┬───────────┘                          │              │
│               │ process                              │              │
│               ▼                                      ▼              │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │                    PostgreSQL                                   │ │
│  │  users │ natal_charts │ products │ astro_rules │ requests_log  │ │
│  └────────────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 Технологический стек

| Компонент | Технология |
|-----------|-----------|
| Язык | Go 1.25+ |
| HTTP роутер | go-chi/chi v5 |
| БД | PostgreSQL 15+ (pgx/v5, raw SQL, JSONB) |
| Брокер сообщений | NATS JetStream |
| AI | Yandex GPT (YandexGPT 3) через API |
| Логирование | zap (structured JSON) |
| Миграции | goose |
| Тесты | testify + gomock + testcontainers |

---

## 3. Схема базы данных

### 3.1 Таблица `users`

```sql
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       VARCHAR(255) NOT NULL UNIQUE,  -- Telegram ID
    encrypted_dob BYTEA NOT NULL,                -- AES-256-GCM
    consent_given BOOLEAN NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### 3.2 Таблица `natal_charts` (планируется)

```sql
CREATE TABLE natal_charts (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        VARCHAR(255) NOT NULL UNIQUE REFERENCES users(user_id),
    sun_sign       VARCHAR(50),
    moon_sign      VARCHAR(50),
    venus_sign     VARCHAR(50),
    mars_sign      VARCHAR(50),
    houses         JSONB,    -- {"1":"Aries","2":"Taurus",...,"12":"Pisces"}
    dominant_element VARCHAR(20),
    calculated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### 3.3 Таблица `products`

```sql
CREATE TABLE products (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ext_product_id VARCHAR(255) NOT NULL UNIQUE,  -- ID из RetailCRM
    title          VARCHAR(500) NOT NULL,
    price          NUMERIC(10,2) NOT NULL,
    url            TEXT NOT NULL,
    images         TEXT[] NOT NULL DEFAULT '{}',
    tags           JSONB NOT NULL DEFAULT '[]',   -- ["red","sport","premium"]
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_products_tags ON products USING GIN (tags jsonb_path_ops);
```

### 3.4 Таблица `astro_rules`

```sql
CREATE TABLE astro_rules (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(255) NOT NULL,
    astro_condition JSONB NOT NULL,  -- {"planet":"Venus","sign":"Taurus"}
    product_tags    JSONB NOT NULL DEFAULT '[]',
    priority        INTEGER NOT NULL DEFAULT 100,
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

### 3.5 Таблица `requests_log`

```sql
CREATE TABLE requests_log (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    request_id     UUID NOT NULL UNIQUE,
    user_id        VARCHAR(255) NOT NULL,
    scenario       VARCHAR(50) NOT NULL,   -- personal_style | perfect_gift
    status         VARCHAR(20) NOT NULL,   -- accepted|processing|completed|failed
    attempt_count  INTEGER NOT NULL DEFAULT 0,
    error_reason   TEXT,
    result_payload JSONB,
    created_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

---

## 4. API Contract

### 4.1 Client API (для бота)

Все эндпоинты требуют `Authorization: Bearer {bot_api_key}`.

#### `POST /api/v1/astro/profile`

Сохранение профиля пользователя. Дата рождения шифруется AES-256-GCM.

**Request:**
```json
{
  "user_id": "123e4567-e89b-12d3-a456-426614174000",
  "birth_date": "1990-05-15",
  "birth_place": "Moscow",
  "consent_given": true
}
```

**Response `202 Accepted`:**
```json
{ "request_id": "uuid-v4" }
```

#### `POST /api/v1/astro/recommend`

Инициация генерации персональных рекомендаций.

**Request:**
```json
{
  "user_id": "uuid-v4",
  "scenario": "personal_style",
  "context": {
    "triggers": ["Венера в Тельце", "Полнолуние"],
    "gender": "female",
    "occasion": "work"
  },
  "mode": "async"
}
```

| Поле | Значения |
|------|---------|
| `scenario` | `personal_style`, `perfect_gift` |
| `mode` | `async` (default, 202), `sync` (200, timeout 5s) |

**Response `202 Accepted` (async):**
```json
{ "request_id": "uuid-v4" }
```

**Webhook payload** (отправляется POST на `webhook_url` бота после генерации):
```json
{
  "request_id": "uuid-v4",
  "status": "completed",
  "result": "Венера в Тельце наделяет вас тонким вкусом...",
  "recommended_items": [
    {
      "product_id": "PROD-123",
      "title": "Шёлковый платок",
      "price": 4990.00,
      "url": "https://shop.example.com/products/123",
      "images": ["https://cdn.example.com/img1.jpg"],
      "reason": "Классика и изящество соответствуют Тельцу"
    }
  ]
}
```

#### `POST /api/v1/feedback`

```json
{
  "request_id": "uuid-v4",
  "rating": "positive",
  "comment": "Отличная подборка!"
}
```

### 4.2 Admin API (защищён Bearer-токеном)

| Метод | Путь | Описание |
|-------|------|---------|
| `GET` | `/api/v1/admin/rules` | Список правил (limit, offset, is_active) |
| `POST` | `/api/v1/admin/rules` | Создать правило |
| `PUT` | `/api/v1/admin/rules/{id}` | Обновить правило |
| `DELETE` | `/api/v1/admin/rules/{id}` | Деактивировать правило |
| `GET` | `/api/v1/admin/products` | Список товаров (limit, offset, tag, search) |
| `POST` | `/api/v1/admin/catalog/import` | Импорт из CSV |
| `GET` | `/api/v1/admin/logs` | Лог генераций (status, date_from, date_to) |
| `GET` | `/api/v1/` | Health check |

---

## 5. Асинхронный workflow (NATS JetStream)

### 5.1 Поток обработки рекомендации

```
Бот → POST /recommend
         │
         ├─ создать requests_log (status=accepted)
         ├─ publish → NATS: astro.events.recommend
         └─ ← 202 { request_id }

                    ↓ Consumer забирает
         ┌──────────────────────────────────────┐
         │  RecommendProcessor                  │
         │                                      │
         │  1. Получить user (расшифровать DOB) │
         │  2. Получить natal_chart (из cache)  │
         │  3. Match astro_rules → tags         │
         │  4. SELECT products WHERE tags?|...  │
         │  5. BuildPrompt(scenario, profile)   │
         │  6. YandexGPT.Generate(prompt)       │
         │  7. UpdateStatus(completed, result)  │
         │  8. POST webhook_url → боту          │
         └──────────────────────────────────────┘
```

### 5.2 Retry и DLQ

```
Попытка 1 ──┐ ошибка → NAK + delay 5s
Попытка 2 ──┤ ошибка → NAK + delay 30s
Попытка 3 ──┤ ошибка → NAK + delay 5m
Попытка 4 ──┤ ошибка → NAK + delay 1h
Попытка 5 ──┘ ошибка → DLQ (astro.dlq)
                         └─ requests_log status=failed
                         └─ Admin видит в /admin/logs
```

### 5.3 Конфигурация consumers

| Consumer | Subject | MaxDeliver |
|----------|---------|-----------|
| `astro-profile-worker` | `astro.events.profile` | 5 |
| `astro-recommend-worker` | `astro.events.recommend` | 5 |

---

## 6. Rule-Engine

### 6.1 Как работает маппинг

```
Астро-триггер (из запроса бота)
  → "Венера в Тельце"
  → SELECT astro_rules WHERE name = 'Венера в Тельце' AND is_active=true
  → product_tags: ["luxury", "comfort", "classic"]

Все совпавшие теги →
  → SELECT * FROM products WHERE tags ?| ARRAY['luxury','comfort','classic']
  → ORDER BY RANDOM() LIMIT 10
```

### 6.2 Пример правила

```json
{
  "name": "Венера в Тельце",
  "astro_condition": { "planet": "Venus", "sign": "Taurus" },
  "product_tags": ["luxury", "comfort", "classic", "high-quality"],
  "priority": 10,
  "is_active": true
}
```

---

## 7. Безопасность

### 7.1 Шифрование даты рождения

- Алгоритм: **AES-256-GCM** (authenticated encryption)
- Ключ: 32 байта, base64-encoded, через переменную `ENCRYPTION_KEY`
- Ключ никогда не хранится в коде или git-репозитории
- Расшифровка только в оперативной памяти (Go-слой)

### 7.2 Consent Management

- `consent_given = false` → дата рождения **не сохраняется** в БД
- Данные живут только в памяти на время сессии
- Реализовано право на забвение (DELETE users + requests_log)

### 7.3 API Authentication

- Bot API: Bearer-токен (статический, из env)
- Admin API: Bearer-токен (`ADMIN_TOKEN` из env)

---

## 8. Переменные окружения

| Переменная | Описание | Обязательна |
|-----------|---------|-------------|
| `ENCRYPTION_KEY` | AES ключ (32 байта, base64) | ✅ |
| `ADMIN_TOKEN` | Токен для Admin API | ✅ |
| `DB_HOST` / `DB_PORT` / `DB_USER` / `DB_PASSWORD` / `DB_NAME` | PostgreSQL | ✅ |
| `NATS_HOST` / `NATS_PORT` | NATS JetStream | ✅ |
| `AI_BASE_URL` | URL Yandex GPT API | ✅ |
| `AI_API_KEY` | Ключ Yandex Cloud | ✅ |
| `AI_MODEL_URL` | URI модели YandexGPT | ✅ |
| `LOG_LEVEL` | `debug` / `info` / `error` | нет (default: info) |

---

## 9. Производительность (SLA)

| Метрика | Цель |
|---------|------|
| HTTP API latency P95 | < 100ms |
| HTTP API latency P99 | < 200ms |
| Throughput | 1000 req/s на инстанс |
| Время генерации E2E | 15–45 сек (зависит от AI) |
| Доступность | 99.9% |

---

## 10. Дорожная карта (MVP → Post-MVP)

### Phase 1 ✅ DONE — Core infrastructure
- PostgreSQL + NATS + DLQ + retry
- POST /astro/profile, POST /astro/recommend (async + sync)
- Admin Rules CRUD
- Шифрование DOB
- Unit + integration тесты

### Phase 2 🔄 IN PROGRESS — Полнота API
- [ ] Внешний Astro API клиент + таблица `natal_charts`
- [ ] Webhook доставка результата боту
- [ ] GET /admin/products + POST /admin/catalog/import
- [ ] GET /admin/logs
- [ ] POST /feedback
- [ ] GET /admin/requests/{id} (polling статуса)

### Phase 3 🔜 NEXT — RetailCRM + Admin Panel
- [ ] Синхронизация каталога из RetailCRM
- [ ] Admin Panel (React + Ant Design)
- [ ] Full E2E тесты + load testing (k6)
- [ ] Prometheus метрики

### Phase 4 🔜 FUTURE — AI Optimization
- [ ] A/B тестирование промптов
- [ ] Авто-оптимизация Rule-Engine по feedback
- [ ] WhatsApp / Viber каналы
