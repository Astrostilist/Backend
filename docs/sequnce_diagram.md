
### 1. Сценарий: Запрос астропрофиля (Асинхронный)
*Эндпоинт: `POST /api/v1/astro/profile`*
*Логика:* API принимает запрос, сохраняет пользователя, публикует задачу в NATS и сразу возвращает `202 Accepted`. Воркер считает профиль и шлет результат в Webhook бота.

```mermaid
sequenceDiagram
    autonumber
    participant Bot as Telegram Bot (Client)
    participant API as Backend API (Go/Chi)
    participant DB as PostgreSQL (users)
    participant NATS as NATS JetStream<br/>(Stream: astro.events)
    participant Worker as Profile Worker
    participant Astro as External Astro API
    participant BotWebhook as Client Webhook

    Bot->>API: POST /api/v1/astro/profile<br/>{user_id, dob, consent, webhook_url}
    
    rect rgb(240, 248, 255)
        note right of API: Sync DB Write only
        API->>DB: INSERT/UPDATE users<br/>(encrypted_dob, consent)
    end

    API->>NATS: Publish Message<br/>(Subject: astro.events.profile)
    
    API-->>Bot: 202 Accepted<br/>{status='pending', request_id}

    NATS->>Worker: Deliver Message
    
    Worker->>DB: SELECT encrypted_dob FROM users
    Worker->>Worker: Decrypt DOB
    
    Worker->>Astro: GET /calculate-chart<br/>{dob}
    Astro-->>Worker: Return Astro Data
    
    Worker->>DB: UPDATE users SET profile_cache=... (Optional)
    
    Worker->>BotWebhook: POST {webhook_url}<br/>{status='success', profile_data}
    
    Worker->>NATS: Ack Message
```

**Ключевые моменты:**
*   **DB:** Таблица `users`.
*   **NATS:** Используется стрим `astro.events` (или отдельный `astro.profiles`).
*   **Response:** Мгновенный `202 Accepted`.
*   **Result:** Отправляется асинхронно на `webhook_url`.

---

### 2. Сценарий: Генерация рекомендации (Асинхронный)
*Эндпоинт: `POST /api/v1/astro/recommend`*
*Логика:* Без изменений, полностью асинхронная.

```mermaid
sequenceDiagram
    autonumber
    participant Bot as Telegram Bot (Client)
    participant API as Backend API (Go/Chi)
    participant DB as PostgreSQL<br/>(requests_log, products, astro_rules)
    participant NATS as NATS JetStream<br/>(Stream: astro.events)
    participant Worker as Recommendation Worker
    participant OpenAI as OpenAI API
    participant BotWebhook as Client Webhook

    Bot->>API: POST /api/v1/astro/recommend<br/>{user_id, scenario, webhook_url}
    
    API->>DB: INSERT INTO requests_log<br/>(status='pending')
    API->>NATS: Publish Message<br/>(Subject: astro.events.recommend)
    API-->>Bot: 202 Accepted<br/>{request_id, status='pending'}

    NATS->>Worker: Deliver Message
    
    Worker->>DB: UPDATE requests_log SET status='processing'
    
    rect rgb(255, 250, 240)
        note right of Worker: Business Logic
        Worker->>DB: SELECT * FROM users WHERE user_id=?
        Worker->>DB: SELECT * FROM astro_rules
        Worker->>DB: SELECT * FROM products (GIN Search)
        Worker->>OpenAI: Chat Completion
        OpenAI-->>Worker: JSON Response
    end

    alt Success
        Worker->>DB: UPDATE requests_log<br/>SET status='success', result_payload=...
        Worker->>BotWebhook: POST {webhook_url}<br/>{status='success', result}
        Worker->>NATS: Ack Message
    else Error
        Worker->>DB: UPDATE requests_log<br/>SET status='retry', error_reason=...
        Worker->>NATS: Nak Message (Exponential Backoff)
    end
```

**Ключевые моменты:**
*   **DB Tables:** `requests_log`, `users`, `astro_rules`, `products`.
*   **NATS:** Retry логика через `Nak` с экспоненциальной задержкой.
*   **Webhook:** Единый механизм доставки результатов для обоих сценариев.

### Сводная таблица потоков данных

| Эндпоинт | Метод | Действие API | NATS Subject | Таблицы БД (Write) | Ответ Клиенту | Доставка Результата |
| :--- | :--- | :--- | :--- | :--- | :--- | :--- |
| `/astro/profile` | POST | Save User, Publish | `astro.events.profile` | `users` | 202 Accepted | Webhook (Async) |
| `/astro/recommend` | POST | Create Log, Publish | `astro.events.recommend` | `requests_log` | 202 Accepted | Webhook (Async) |
| `/feedback` | POST | Save Feedback | None (Sync) | `requests_log` (update) | 200 OK | N/A |

**Примечание:** Эндпоинт `/feedback` остается синхронным, так как это простая запись статуса и не требует тяжелых вычислений или внешних вызовов.
