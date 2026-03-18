# GoSystem — Task Tracker API

Простой и понятный pet‑backend на Go: хранит задачи в Postgres и дает удобный REST API.

## Что делает
- Создает задачи, обновляет поля и статус.
- Поддерживает проекты и участников проектов.
- Пользователи и назначение исполнителей.
- Теги, чек‑лист задач, комментарии и активность.
- Фильтры по статусу, приоритету, проекту, тегу и поиску.
- Отдельные списки: overdue и due‑soon.
- Bulk‑изменение статуса для пачки задач.
- Статистика по задачам и по конкретному проекту.
- `/health` для проверки сервиса.

## Структура
- `cmd/api`: запуск сервера.
- `internal/httpapi`: HTTP‑обработчики.
- `internal/store`: работа с Postgres.

## Быстрый старт (Docker)
1. Поднять Postgres:
   - `docker compose -f deployments/docker-compose.yml up -d`
2. Накатить миграции:
   - `psql "postgres://gosystem:gosystem@localhost:5432/gosystem?sslmode=disable" -f migrations/001_init.sql`
3. Запустить API:
   - `go run ./cmd/api`

## API
Base URL: `http://localhost:8080`

Endpoints:
- `POST /tasks`
- `GET /tasks`
- `GET /tasks/{id}`
- `PATCH /tasks/{id}`
- `DELETE /tasks/{id}`
- `PATCH /tasks/{id}/status`
- `GET /tasks/{id}/comments`
- `POST /tasks/{id}/comments`
- `GET /tasks/{id}/assignees`
- `POST /tasks/{id}/assignees`
- `DELETE /tasks/{id}/assignees/{user_id}`
- `GET /tasks/{id}/checklist`
- `POST /tasks/{id}/checklist`
- `PATCH /tasks/{id}/checklist/{item_id}`
- `DELETE /tasks/{id}/checklist/{item_id}`
- `GET /tasks/{id}/activity`
- `GET /tasks/overdue`
- `GET /tasks/due-soon`
- `POST /tasks/bulk/status`
- `POST /projects`
- `GET /projects`
- `GET /projects/{id}`
- `GET /projects/{id}/members`
- `POST /projects/{id}/members`
- `GET /projects/{id}/stats`
- `POST /users`
- `GET /users`
- `GET /users/{id}`
- `GET /tags`
- `GET /stats`
- `GET /health`

OpenAPI спецификация: `api/openapi.yaml`.

## Config
Все настройки — через переменные окружения. Пример в `configs/docker.env`.
