---
id: messenger-dev-setup
title: Локальный запуск messenger-api
team: t-messenger
topic: dev-setup
---

> Учебная инструкция: составлена для демонстрации buddy, не является регламентом Авито.

## Что нужно
Go той же версии, что в `go.mod`, Docker и доступ к репозиторию `messenger-api` в Bitbucket.

## Запуск
1. Склонируй репозиторий и выполни `make deps`.
2. `make dev` поднимает Postgres и Kafka в Docker и запускает сервис с конфигом `config/local.yaml`.
3. Проверка: `curl localhost:8080/healthz` отвечает `ok`.

## Если не собирается
Чаще всего не хватает доступа к приватным модулям: проверь `GOPRIVATE` и токен Bitbucket. Если не помогло,
спроси наставника, он знает про сборку больше всех.
