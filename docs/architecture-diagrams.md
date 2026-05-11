# Architecture Diagrams

## 1. Общая архитектура системы

```mermaid
graph TB
    subgraph External["Внешние системы"]
        Bot["Telegram Bot\n(заказчик)"]
        CRM["RetailCRM\n(каталог товаров)"]
        AI["YandexGPT / AlisaAI\n(генерация текста)"]
        Admin["Admin Panel\n(браузер)"]
    end

    subgraph Backend["Backend (astroapi)"]
        Router["HTTP Router (chi)\nPOST /astro/profile\nPOST /astro/recommend\nCRUD /admin/rules\nGET /admin/logs"]

        subgraph NATS["NATS JetStream"]
            Stream["astro_events stream\nRetention: WorkQueue"]
            DLQ["astro_dlq stream\nRetention: Limits"]
            ProfWorker["profile-worker\nconsumer"]
            RecWorker["recommend-worker\nconsumer"]
        end

        CRMSync["RetailCRM\nSync Worker"]
        AlisaClient["Alisa AI\nClient"]
    end

    subgraph DB["PostgreSQL"]
        Users["users"]
        Products["products"]
        Rules["astro_rules"]
        ReqLog["requests_log"]
    end

    Bot -->|"REST HTTP"| Router
    Admin -->|"REST HTTP + Bearer token"| Router
    CRM -->|"API polling"| CRMSync

    Router -->|"publish"| Stream
    Stream --> ProfWorker
    Stream --> RecWorker
    Stream -->|"max retries exceeded"| DLQ

    ProfWorker -->|"save user"| Users
    ProfWorker -->|"update status"| ReqLog

    RecWorker -->|"match triggers→tags"| Rules
    RecWorker -->|"get user profile"| Users
    RecWorker -->|"generate text"| AlisaClient
    RecWorker -->|"save result"| ReqLog

    AlisaClient -->|"HTTP"| AI
    CRMSync -->|"upsert"| Products

    Router -->|"CRUD rules"| Rules
    Router -->|"read logs"| ReqLog
```

---

## 2. Флоу бота — сохранение профиля

```mermaid
sequenceDiagram
    participant Bot as Telegram Bot
    participant API as HTTP API
    participant NATS as NATS JetStream
    participant Worker as Profile Worker
    participant DB as PostgreSQL

    Bot->>API: POST /api/v1/astro/profile\n{user_id, birth_date, consent_given}
    API->>API: Validate (uuid, date format)
    API->>DB: INSERT requests_log\n(status=accepted)
    API->>NATS: Publish astro.events.profile\n{request_id, profile}
    API-->>Bot: 202 Accepted\n{request_id}

    Note over NATS,Worker: Async processing

    NATS->>Worker: Consume message
    Worker->>Worker: Unmarshal + Validate payload

    alt consent_given=true
        Worker->>DB: UPSERT users\n(birth_date encrypted AES-256)
        Worker->>DB: UPDATE requests_log\n(status=completed)
    else consent_given=false
        Note over Worker: birth_date NOT saved\n(GDPR / ФЗ-152)
        Worker->>DB: UPDATE requests_log\n(status=completed)
    end

    alt error occurred
        Worker->>DB: UPDATE requests_log\n(status=failed, error_msg)
        Worker->>NATS: NAK → retry (backoff: 5s/30s/5m/1h)
        Note over NATS: After 5 retries → astro_dlq
    end
```

---

## 3. Флоу бота — получение рекомендации (async)

```mermaid
sequenceDiagram
    participant Bot as Telegram Bot
    participant API as HTTP API
    participant NATS as NATS JetStream
    participant Worker as Recommend Worker
    participant DB as PostgreSQL
    participant AI as AlisaAI (YandexGPT)

    Bot->>API: POST /api/v1/astro/recommend\n{user_id, scenario, context, mode="async"}
    API->>API: Validate request
    API->>DB: INSERT requests_log (status=accepted)
    API->>NATS: Publish astro.events.recommend\n{request_id, recommend}
    API-->>Bot: 202 Accepted {request_id}

    Note over Bot: Polling result separately

    NATS->>Worker: Consume message
    Worker->>DB: GET users (birth_date)
    Worker->>DB: Match astro_rules\n(triggers → product tags)
    Worker->>Worker: BuildPrompt(scenario, astro_profile, tags)
    Worker->>AI: POST /generate {prompt}
    AI-->>Worker: Generated text
    Worker->>DB: UPDATE requests_log\n(status=completed, result_json)

    Bot->>API: GET /api/v1/requests/{request_id}
    API->>DB: SELECT requests_log
    API-->>Bot: {status, result, tags}
```

---

## 4. Флоу бота — получение рекомендации (sync)

```mermaid
sequenceDiagram
    participant Bot as Telegram Bot
    participant API as HTTP API
    participant DB as PostgreSQL
    participant AI as AlisaAI

    Bot->>API: POST /api/v1/astro/recommend\n{user_id, scenario, mode="sync"}
    API->>API: Validate + create request_id
    API->>DB: INSERT requests_log (status=accepted)

    Note over API: ctx timeout = 5s

    API->>DB: GET users (birth_date)
    API->>DB: Match astro_rules (triggers → tags)
    API->>API: BuildPrompt(scenario, astro_profile, tags)
    API->>AI: POST /generate {prompt}

    alt success (< 5s)
        AI-->>API: Generated text
        API->>DB: UPDATE requests_log (status=completed)
        API-->>Bot: 200 OK {request_id, result, tags, status}
    else timeout (>= 5s)
        API->>DB: UPDATE requests_log (status=failed)
        API-->>Bot: 504 Gateway Timeout
    else user not found
        API->>DB: UPDATE requests_log (status=failed)
        API-->>Bot: 404 Not Found
    end
```

---

## 5. Флоу Admin Panel — управление правилами (Rule Engine)

```mermaid
sequenceDiagram
    participant Admin as Admin Browser
    participant API as HTTP API\n(AdminAuthMiddleware)
    participant DB as PostgreSQL (astro_rules)

    Note over Admin,API: All requests require\nAuthorization: Bearer <ADMIN_TOKEN>

    Admin->>API: GET /api/v1/admin/rules?limit=50&offset=0&is_active=true
    API->>DB: SELECT astro_rules WHERE is_active=true LIMIT 50
    API-->>Admin: 200 {items[], total_count, limit, offset}

    Admin->>API: POST /api/v1/admin/rules\n{name, astro_condition, product_tags, priority}
    API->>API: Validate (name required,\nastro_condition required,\npriority >= 0)
    API->>API: normalizeTags (lowercase, dedup)
    API->>DB: INSERT astro_rules
    API-->>Admin: 201 {rule}

    Admin->>API: PUT /api/v1/admin/rules/{id}\n{name, astro_condition, product_tags}
    API->>DB: UPDATE astro_rules WHERE id={id}
    alt not found
        API-->>Admin: 404 Not Found
    else ok
        API-->>Admin: 200 {updated_rule}
    end

    Admin->>API: DELETE /api/v1/admin/rules/{id}
    Note over API: Soft delete — sets is_active=false
    API->>DB: UPDATE astro_rules SET is_active=false
    API-->>Admin: 200 {deactivated_rule}
```

---

## 6. Rule Engine — маппинг триггеров на теги

```mermaid
flowchart TD
    Input["Входные триггеры\n['Полнолуние', 'Стрелец']"]

    subgraph RuleEngine["Rule Engine (PostgreSQL JSONB)"]
        Rules["astro_rules\nname | astro_condition | product_tags | priority | is_active"]
        Query["SELECT rules WHERE\nastro_condition @> triggers\nORDER BY priority DESC"]
    end

    Merge["Merge product_tags\n(deduplicated)"]
    Output["Итоговые теги\n['romantic', 'fire-sign', 'luxury']"]

    Input --> Query
    Rules --> Query
    Query --> Merge
    Merge --> Output

    Output -->|"used in"| Prompt["BuildPrompt(scenario, astro_profile, tags)"]
    Prompt --> AI["AlisaAI /generate"]
    AI --> Text["Персональный текст рекомендации"]
```

---

## 7. NATS JetStream — обработка ошибок и DLQ

```mermaid
flowchart LR
    Publish["Publish\nastro.events.*"]

    subgraph Stream["astro_events (WorkQueue)"]
        Msg["Message"]
    end

    subgraph Consumers["Workers"]
        PW["profile-worker"]
        RW["recommend-worker"]
    end

    subgraph Retry["Retry backoff"]
        R1["5s"]
        R2["30s"]
        R3["5m"]
        R4["1h"]
    end

    DLQ["astro_dlq\n(LimitsPolicy)\nmax 100k msgs / 100MB"]
    DB["PostgreSQL\nrequests_log\nstatus=failed"]

    Publish --> Msg
    Msg --> PW
    Msg --> RW

    PW -->|"error → NAK"| R1 --> R2 --> R3 --> R4
    RW -->|"error → NAK"| R1

    R4 -->|"5 retries exceeded"| DLQ
    DLQ -->|"dead letter alert"| DB
```

---

## 8. Компонентная диаграмма — зависимости пакетов

```mermaid
graph TD
    subgraph cmd
        Main["cmd/main.go\nDI wiring + graceful shutdown"]
    end

    subgraph handlers
        ProfH["ProfileHandler"]
        RecH["RecommendHandler"]
        AdminH["AdminRulesHandler"]
        Router["MsgRouter\n(NATS dispatch)"]
        ProcP["ProfileProcessor"]
        ProcR["RecommendProcessor"]
    end

    subgraph domain
        UserRepo["user.Repository"]
        ReqRepo["requests.Repository"]
        RuleRepo["ruleengine.Repository"]
        AlisaGen["alisa.Generator"]
    end

    subgraph infra
        NATS["infrastructure/nats\nJetStreamAdapter"]
        PG["database.PostgresDB"]
        Crypto["crypto\nAES-256-GCM"]
    end

    Main --> ProfH
    Main --> RecH
    Main --> AdminH
    Main --> Router
    Main --> ProcP
    Main --> ProcR

    ProfH --> ReqRepo
    ProfH --> NATS

    RecH --> ReqRepo
    RecH --> UserRepo
    RecH --> RuleRepo
    RecH --> AlisaGen
    RecH --> NATS

    AdminH --> RuleRepo

    ProcP --> UserRepo
    ProcP --> ReqRepo
    ProcP --> Crypto

    ProcR --> UserRepo
    ProcR --> ReqRepo
    ProcR --> RuleRepo
    ProcR --> AlisaGen

    UserRepo --> PG
    ReqRepo --> PG
    RuleRepo --> PG
```
