# Pulse backend

Бэкенд сервиса знакомств Pulse. Сейчас разрабатывается MVP: регистрация по
email/паролю, анкета с проверкой 18+, фотографии, взаимные фильтры, лента,
свайпы и список мэтчей.
Чат, прекращение общения, модерация, поддержка и уведомления относятся к
следующим релизам.

Правила для AI-агента находятся в [AGENTS.md](AGENTS.md). Начальная точка
подробной документации — [AI/README.md](AI/README.md). Решения владельца от
12–13 октября 2026 года записаны в [AI/DECISIONS.md](AI/DECISIONS.md) и имеют
приоритет над первоначальными документами требований.

Текущий `.gitignore` исключает `AGENTS.md` и `AI/`: инструкции доступны локально
и не появятся на другой машине после обычного clone без отдельной передачи.

Общие интерфейсы и разделение T08–T12 между двумя разработчиками описаны в
[docs/PARALLEL_MVP.md](docs/PARALLEL_MVP.md); этот документ хранится в Git.

## Текущее состояние

Работают health checks и базовые logging/recovery/metrics middleware. Написаны
сервисы и HTTP-обработчики auth/session, анкеты/preferences/sleep, фото, ленты,
свайпов и списка мэтчей;
приватные маршруты защищены user access JWT, зависимости собраны в `cmd/api`.
PostgreSQL-запросы реализованы: регистрация, сессии, анкеты, фото, лента,
свайпы и список мэтчей работают с реальной БД. Все 30 запросов репозиториев
и 8 запросов тестовых данных собраны с пояснениями в
[docs/SQL_QUERIES.md](docs/SQL_QUERIES.md).

На изолированных PostgreSQL 18 и MinIO прошёл полный прогон
`go test -count=1 -race -tags=integration ./...`. Проверены взаимные фильтры,
исключения, keyset pagination, конкурентные refresh/загрузки/встречные лайки,
ownership и HTTP-сценарий регистрация → анкета/preferences → фото → лента →
взаимный мэтч → refresh/logout. Фото из ленты и списка пар доступны по
подписанным URL реального S3-адаптера.

Для обычных PostgreSQL-тестов задайте `PULSE_TEST_DATABASE_URL` отдельного
тестового сервера с правом CREATE DATABASE. Для tagged S3-тестов нужны
`S3_ENDPOINT`, `S3_PUBLIC_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY`
изолированного MinIO с созданным bucket. Без этих настроек соответствующие
интеграционные тесты пропускаются. Команды и ограничения — в справочнике SQL.

Все операции MVP в OpenAPI помечены `implemented`. Чат, закрытие мэтча и
модерация остаются отложенными `501`-операциями. Полная приёмка MVP ещё требует
frontend QA, измерений скорости и оставшихся сценариев отказов/перезапуска;
подробности — [AI/STATUS.md](AI/STATUS.md).

## Структура

```text
cmd/api/                       сборка зависимостей и graceful shutdown
internal/app/                  сценарии приложения и интерфейсы сервисов
  auth/                        регистрация, вход и lifecycle сессий
  adminauth/                   заготовка авторизации администратора после MVP
  profile/                     анкета и предпочтения
  media/                       фотографии
  discovery/                   лента рекомендаций
  matching/                    свайпы и мэтчи
  chat/                        будущие отдельные диалоги
  moderation/                  будущие жалобы и действия администратора
internal/domain/               сущности, ограничения и порты хранения
internal/repository/postgres/  PostgreSQL-запросы и транзакции
internal/storage/s3/           S3-адаптер фотографий и интеграционный тест
internal/platform/database/    создание пула подключений PostgreSQL
internal/security/             bcrypt и JWT
internal/transport/http/       HTTP-адаптеры, DTO, маршруты и middleware
api/openapi.yaml               целевой контракт API
migrations/                    историческая и согласованная MVP-схема
AI/                            контекст и инструкции для дальнейшей разработки
```

## Основные правила

- UUID для идентификаторов сущностей. Go module: `github.com/kungrem23/pulse-backend`.
- Регистрация только по email и паролю; возраст 18–120 обязателен при создании
  анкеты. До анкеты users.age = NULL. Дата рождения не собирается.
- `city` обязателен; пол анкеты — `male` или `female`, предпочтение — одно из
  `male`, `female`, `any`.
- Описание до 500 символов; не более 5 фотографий JPEG/PNG/WebP по 10 MiB
  (10 485 760 байт). В БД хранится ключ объекта, в API выдаётся URL.
- Лента учитывает фильтры обоих людей и исключает самого пользователя,
  спящие/заблокированные/незавершённые анкеты, оценённые анкеты и все мэтчи.
- Сохранение свайпа и создание единственного мэтча для пары атомарны.
  Повторный мэтч невозможен, в том числе после прекращения общения.
- После MVP отдельный диалог создаётся для мэтча; сообщения до 2000 символов.
  Закрытие сохраняет историю для участников и навсегда запрещает новые сообщения.
- Пароли — bcrypt; сессии, refresh rotation/logout и ошибки — по OpenAPI.

## Локальная конфигурация

Для запуска нужны Docker Engine и Docker Compose v2. Значений по
умолчанию достаточно для local-стенда. Для своих настроек скопируйте
`.env.example` в `.env`; существующий `.env` не перезаписывайте. Compose
читает этот файл для подстановки; Go-приложение на хосте само `.env` не
загружает.

Подключение к базе задаётся отдельными `DB_HOST`, `DB_PORT`, `DB_USER`,
`DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE`. `DATABASE_URL` не является входной
переменной. В Compose host базы — `db`, endpoint хранилища —
`http://minio:9000`; при запуске Go на хосте используйте `localhost`.
Примерные credentials предназначены только для local/test.

Для хранилища используются `S3_ENDPOINT`, `S3_PUBLIC_ENDPOINT`, `S3_BUCKET`,
`S3_ACCESS_KEY`, `S3_SECRET_KEY`, `S3_REGION` и `S3_PRESIGN_TTL`.
`S3_ENDPOINT` доступен самому backend, а `S3_PUBLIC_ENDPOINT` должен быть
доступен frontend: адаптер подписывает URL для него. Readiness проверяет и
PostgreSQL, и наличие настроенного bucket.

`CORS_ALLOWED_ORIGINS` — точные origins через запятую; пустое значение выключает
CORS, wildcard и credentials запрещены. Например, для frontend:
`http://localhost:5173`. Auth ограничивается отдельно на каждый маршрут и IP
соединения: технические defaults `AUTH_RATE_LIMIT_REQUESTS=30`,
`AUTH_RATE_LIMIT_WINDOW=1m`, `AUTH_RATE_LIMIT_MAX_KEYS=100000`.
Лимитер локален для процесса и не доверяет `X-Forwarded-For`; за reverse proxy
клиенты делят бюджет IP прокси. Пороги можно переопределить в окружении.

```bash
docker compose up --build --detach
curl --fail http://localhost:8080/health/live
curl --fail http://localhost:8080/health/ready
```

Доступны API `http://localhost:8080`, PostgreSQL `localhost:5432`, S3 API
`http://localhost:9000` и MinIO Console `http://localhost:9001`. `db-schema` и
`minio-init` должны завершиться с кодом 0. Остановка без удаления
данных: `docker compose down`.

## Миграции

`initial_001.sql` — сохранённая историческая схема. `decisions_002.sql`
заменяет только её пустые таблицы согласованной MVP-схемой. На новом
Compose volume файлы применяются как `initial_001 → decisions_002 → profile_age_003`;
`db-schema` проверяет результат до старта API.

Если есть хотя бы одна legacy-запись, миграция завершается ошибкой
без изменения данных. Не удаляйте volume и не обходите guard: нужен
отдельный согласованный перенос. Для не-Compose пустой базы порядок явный:

```bash
psql -X -v ON_ERROR_STOP=1 -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -f migrations/initial_001.sql
psql -X -v ON_ERROR_STOP=1 -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -f migrations/decisions_002.sql
psql -X -v ON_ERROR_STOP=1 -h "$DB_HOST" -p "$DB_PORT" -U "$DB_USER" -d "$DB_NAME" -f migrations/profile_age_003.sql
```

`profile_age_003.sql` снимает NOT NULL с users.age, сохраняя CHECK 18–120
и все существующие данные. Для БД с уже применённой `decisions_002` нужно
однократно применить только новую миграцию перед запуском обновлённого backend.
Инициализация Compose не применяет новые файлы к существующему volume.
Для выбранной локальной Compose-БД:

```bash
docker compose exec -T db sh -c 'exec psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < migrations/profile_age_003.sql
```

Затем пересоберите и перезапустите backend. `db-schema` требует все три версии.
Повторное применение миграции завершается ошибкой; исторические миграции
не запускать повторно. Для другой БД сначала явно выберите целевое подключение.

Порядок работы с существующей БД — [AI/DATABASE.md](AI/DATABASE.md). Общий
runner для непустых баз и rollback/down-миграции не реализованы.

## Проверки

```bash
go test ./...
go vet ./...
docker compose config --quiet
docker compose -f docker-compose.yml -f docker-compose.test.yml config --quiet
./scripts/verify-environment.sh
git diff --check
```

`verify-environment.sh` использует только изолированный Compose-проект
`pulse-backend-t01-test`, проверяет build/runtime, health, SQL-инварианты,
реальные put/get/delete через S3-адаптер, безопасную реакцию readiness на
отказ MinIO, сохранность PostgreSQL/MinIO после restart и legacy guard, затем
удаляет этот тестовый проект с volumes. Проходящие проверки T01/T07 не означают
готовность HTTP-управления фотографиями: оно относится к T08.
