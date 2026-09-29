# Структура

```text
cmd/api/                       точка сборки приложения и graceful shutdown
internal/app/                  сценарии приложения и их контракты
  auth/                        регистрация и вход
  profile/                     анкета и предпочтения
  media/                       фотографии
  discovery/                   лента рекомендаций
  matching/                    свайпы и мэтчи
  chat/                        сообщения
  moderation/                  жалобы и действия администратора
internal/domain/               сущности и порты хранения
internal/repository/postgres/  реализации репозиториев PostgreSQL
internal/storage/s3/           реализация хранилища фотографий
internal/platform/database/    создание пула подключений
internal/transport/http/       HTTP-адаптеры по тем же сценариям
  httpx/                       общие JSON-ответы и ошибки
  middleware/                  logging, recovery и metrics
migrations/                    SQL-схема и rollback
deploy/                        мониторинг
```

Папки в `transport/http` знают о JSON, URL и HTTP-кодах. Одноимённые папки
в `app` описывают бизнес-сценарии и не зависят от HTTP. `router.go` только
соединяет обработчики, маршруты и middleware.

## API-контуры

- `/api/v1/auth/*` — регистрация и вход пользователя;
- `/api/v1/profile`, `/preferences`, `/photos` — анкета;
- `/api/v1/feed`, `/swipes`, `/matches` — рекомендации и мэтчи;
- `/api/v1/matches/{id}/messages` — чат;
- `/api/v1/reports` — жалобы;
- `/api/v1/admin/*` — модерация, блокировка, сообщения и уведомления;
- `/health/live`, `/health/ready` — Kubernetes/Docker health checks.

Пока бизнес-маршруты намеренно отвечают `501 Not Implemented`: это контракт и
места расширения. Health checks реализованы. Следующий вертикальный срез —
регистрация/авторизация, CRUD анкеты, затем свайп с созданием мэтча в одной
транзакции.

## Важные решения по схеме

- Возраст не хранится: хранится `birth_date`, иначе возраст устаревает.
- Лайк и дизлайк объединены в `swipes.is_like`.
- В мэтче меньший UUID всегда `person1_id`, поэтому пара уникальна.
- Сообщение принадлежит мэтчу, что упрощает проверку доступа к чату.
- Фото хранит только ключ объекта S3, а не временную публичную ссылку.
- Пароли должны попадать в БД только как Argon2id/bcrypt hash.

## Проверки

```bash
go test ./...
go vet ./...
docker compose config
```
