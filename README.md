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
