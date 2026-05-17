# Backend

### Generate key with `make generate-key` or manually: 32 bytes base64 encoded
ENCRYPTION_KEY=
### POST `/api/v1/astro/recommend`
Эндпоинт для получения рекомендаций от нейросети (AlisaAI).

Тело запроса (JSON):
* `user_id` (string, обязательный) — UUID пользователя.
* `scenario` (string, обязательный) — Сценарий генерации. Допустимые значения: `personal_style`, `perfect_gift`.
* `context` (object, опционально) — Дополнительные данные для нейросети.
* `mode` (string, опционально) — Режим работы. Допустимые значения: `sync`, `async` (по умолчанию).

**Режимы работы (`mode`):**
1. **Async (Асинхронный - по умолчанию):** Сервер мгновенно возвращает `202 Accepted` и `request_id`. Результат нужно запрашивать отдельно.
2. **Sync (Синхронный):** Сервер удерживает соединение и ждет ответа от нейросети. Возвращает `200 OK` с готовым результатом. *Внимание: установлен жесткий таймаут 5 секунд, после которого вернется `504 Gateway Timeout`.*

### Process Personal Data UseCase
Обработка персональных данных пользователя с учетом согласия (consent_given).
**Поведение:**
consent_given = true  → данные сохраняются в PostgreSQL
consent_given = false → данные сохраняются только в cache (Memcached)



### AUTH-02: SuperAdmin bootstrap and AUTH-01 login

Создание первой учетной записи SuperAdmin выполняется при развертывании через CLI-команду:

```bash
make init-superadmin
```

или напрямую:

```bash
go run ./cmd/superadmin
```

Команда использует переменные окружения:

```env
SUPERADMIN_EMAIL=admin@example.com
SUPERADMIN_PASSWORD=__CHANGEME_MIN_8_CHARS__
ADMIN_TOKEN=__CHANGEME_ADMIN_TOKEN__
```

`ADMIN_TOKEN` используется как секрет для подписи JWT access token. Статический доступ к admin routes через сырой `ADMIN_TOKEN` больше не используется: сначала нужно получить JWT через login. Повторный запуск bootstrap-команды не создает дубликат SuperAdmin.

После создания SuperAdmin можно пройти авторизацию через AUTH-01:

```bash
curl -X POST http://localhost:8080/api/v1/admin/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@example.com","password":"__CHANGEME_MIN_8_CHARS__"}'
```

Ответ содержит `access_token`:

```json
{
  "message": "Admin authenticated successfully",
  "data": {
    "access_token": "...",
    "token_type": "Bearer",
    "expires_in": 86400
  }
}
```

Токен используется для защищенных admin routes:

```bash
curl http://localhost:8080/api/v1/admin/rules/ \
  -H "Authorization: Bearer <access_token>"
```

### AUTH-03: Client API Bot API Key

Клиентские эндпоинты Telegram-бота защищены статическим ключом из переменной окружения:

```env
BOT_API_KEY=__CHANGEME_BOT_API_KEY__
```

Защищенные маршруты:

* `POST /api/v1/astro/profile`
* `POST /api/v1/astro/recommend`
* `POST /api/v1/feedback`

Каждый запрос от Telegram-бота должен передавать заголовок:

```http
Authorization: Bearer <bot_api_key>
```

Пример:

```bash
curl -X POST http://localhost:8080/api/v1/astro/profile \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer __CHANGEME_BOT_API_KEY__' \
  -d '{"user_id":"...","birth_date":"1990-01-01","birth_place":"Moscow","consent_given":true}'
```

Если `BOT_API_KEY` не задан в окружении либо заголовок отсутствует/неверен, клиентские эндпоинты возвращают `401 Unauthorized`.
