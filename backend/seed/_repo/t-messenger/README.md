# messenger-api

> Учебный репозиторий: составлен для демонстрации buddy, не является кодом Авито.

Сервис чатов мессенджера: создание чатов, отправка и чтение сообщений. HTTP и gRPC API, Postgres для данных,
Kafka для событий «сообщение отправлено».

## Структура

- `cmd/messenger-api/` — точка входа, сборка зависимостей.
- `internal/chat/` — логика чатов и сообщений.
- `internal/storage/` — работа с Postgres.
- `config/` — конфиги окружений.
- `migrations/` — схема БД.

## Запуск локально

```
make deps
make dev
curl localhost:8080/healthz
```

`make dev` поднимает Postgres и Kafka в Docker и запускает сервис с `config/local.yaml`.

## Тесты

`make test` — unit-тесты, `make test-integration` — с Postgres и Kafka в Docker. Подробнее в `CONTRIBUTING.md`.
