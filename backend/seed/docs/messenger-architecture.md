---
id: messenger-architecture
title: Архитектура messenger-api
team: t-messenger
topic: architecture
---

> Учебная инструкция: составлена для демонстрации buddy, не является регламентом Авито.

## Части
- `messenger-api`: HTTP и gRPC API чатов, отправка и чтение сообщений.
- Postgres: чаты, участники, сообщения (партиции по месяцам).
- Kafka: события «сообщение отправлено» для уведомлений и поиска.

## Поток отправки
Клиент вызывает `SendMessage`, сервис пишет сообщение в Postgres и публикует событие в Kafka. Уведомления
читает из Kafka сервис юнита Notifications.

## Где смотреть
Описание API в `api/` репозитория, схема БД в `migrations/`.
