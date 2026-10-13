# Интеграция T10–T12 без SQL

Актуализировано 2026-10-13. Доработаны незавершённые пункты из переданного
владельцем WORKLOG второго разработчика. SQL не написан и не выполнялся,
схема/миграции не менялись. Документ фиксирует результат интеграции, а не
разрешает следующий этап работы с БД.

## Готовая Go-часть

- `SwipeRepository` и `MatchRepository` в `internal/repository/postgres/`
  реализуют общие domain ports и подключены к matching service в `cmd/api`.
  Общий `SearchStateRepository` переиспользован из T09.
- Свайп: стандартные UUID, канонический порядок участников, транзакция
  READ COMMITTED, последовательные блокировки users → profiles → preferences →
  photos, повторные проверки eligibility/истории/взаимных фильтров, запись свайпа
  и опционального мэтча, commit до возврата результата. До трёх попыток только
  для подтверждённых PostgreSQL deadlock/serialization errors; неоднозначные
  ошибки commit/сети не повторяются. Чат не создаётся.
- Список пар: один запрос с партнёром и первым фото, lookahead `limit+1`,
  keyset `(created_at, id)`; безопасный MatchSummary, свежий URL и проверка
  ownership на уровне application. Сон/неготовность партнёра не скрывают пару.
  Неполная страница с `HasMore` считается внутренней ошибкой, а не ошибкой клиента.
- Роутер включает общий JSON для 404/405, CORS, auth limiter и request ID.
  Preflight выполняется до authentication/rate limiting; health и другие
  не-auth маршруты не расходуют auth бюджет. `Allow` и `Retry-After` сохраняются.
- `DecodeJSON` требует `application/json` (допускается charset=utf-8),
  отвергает неизвестные поля, несколько значений и тело свыше 16 KiB.
  Согласно контракту JSON-ошибки дают 400, не новые 413/415.
- Feed и matches используют общий query parser: повреждённая кодировка,
  неизвестные/повторные параметры, пустой cursor и неверный limit дают 400.
- OpenAPI объявляет общие 500, logout 403 и не обещает временный 501 для
  свайпов/списка. Неподключённый MVP matching service даёт безопасный 500;
  закрытие мэтча и остальные поздние функции остаются 501.
- Контрактные проверки добавлены к auth/profile/media/discovery; прежние
  matching-тесты сохранены. Проверяется соответствие ответов схеме OpenAPI,
  ошибки, UUID, публичные DTO и отсутствие внутренних полей.

Работа адаптеров с реальной БД, отсутствие потерянных мэтчей и N+1 зависят
от корректного SQL и пока **не подтверждены**. T10/T11 и сквозной критерий T12
остаются частично выполненными; статусы операций — `partial`.

## Конфигурация HTTP

| Переменная | Техническое значение по умолчанию | Назначение |
| --- | --- | --- |
| `CORS_ALLOWED_ORIGINS` | пусто | Точные http(s) origins через запятую, без wildcard/пути/credentials |
| `AUTH_RATE_LIMIT_REQUESTS` | `30` | Размер burst и скорость пополнения бюджета |
| `AUTH_RATE_LIMIT_WINDOW` | `1m` | Окно пополнения, Go duration не меньше 1s |
| `AUTH_RATE_LIMIT_MAX_KEYS` | `100000` | Максимум отслеживаемых route/IP ключей; 0 использует default |

Ограничены POST register/login/refresh/admin login. Бюджеты раздельные для
каждого маршрута и IP, 429 содержит Retry-After. Значения не являются
продуктовым решением владельца. Конфигурация валидируется до старта;
ошибка CORS не раскрывает случайно переданные credentials.
Переменные передаются backend через Compose, limiter создаётся в `cmd/api`.

Ограничения исходного middleware сохранены: память одного процесса,
недоверенный X-Forwarded-For, полный IPv6-адрес, отсутствие лимита по email.
Распределённый limiter и настройка доверенных прокси не добавлялись.

## Плейсхолдеры для разработчика БД

В `swipe.go` оставлены **9** пустых констант с комментариями параметров/результата:
`lockSwipeUsersQuery`, `lockSwipeProfilesQuery`, `lockSwipePreferencesQuery`,
`lockSwipePhotosQuery`, `swipeHistoryConflictQuery`,
`swipeMutualPreferencesQuery`, `insertSwipeQuery`, `reciprocalLikeQuery`,
`insertSwipeMatchQuery`.
Блокировки обязаны конфликтовать с UPDATE/DELETE и браться в одинаковом
каноническом порядке; это сериализует также пары с общим участником.
Одной UNIQUE-constraint недостаточно. Закрытые пары всегда конфликтуют,
свайпы/мэтчи не перезаписываются и не открываются заново.

В `match.go` — **1** пустой `matchesByUserIDQuery`. Комментарий фиксирует
16 сканируемых столбцов, nullable first-photo projection, владельца, порядок
и keyset/lookahead. SQL должен ограничивать строки мэтчей, а не размноженные
фото, не использовать фильтры поиска и не требовать conversation.
Прежние auth/profile/photo/SearchState/discovery запросы также ещё пустые.

В `matching_test.go` — **4** пустых test-fixture запроса:
блокировка пользователя, чтение исходящего свайпа, чтение всех мэтчей пары,
посев active/closed мэтча с заданным временем. Они используются только
на автоматически созданной и удаляемой тестовой БД.

Все guards возвращают явную внутреннюю ошибку до обращения к pool при
незаполненных repository запросах. Это не in-memory fallback и не успешный
ответ при отсутствии реализации хранилища.

## Проверки

Без выполнения SQL:

```bash
env -u PULSE_TEST_DATABASE_URL go test ./...
env -u PULSE_TEST_DATABASE_URL go test -race ./...
go vet ./...
docker compose config --quiet
docker compose -f docker-compose.yml -f docker-compose.test.yml config --quiet
git diff --check
```

Все перечисленные проверки пройдены. PostgreSQL integration-тесты пропущены,
а не заявлены успешно прошедшими на реальной БД.

После отдельного поручения заполнить SQL подготовлены:

- `TestSwipeRepositoryContractWithPostgres`: весь контракт `swipetest.Run`,
  включая 25 раундов одновременных встречных лайков и повторные запросы.
- `TestMatchRepositoryContractWithPostgres`: весь контракт `matchtest.Run`,
  ownership, первое фото, закрытые/спящие/неполные партнёры, keyset ties.
- `TestMatchingHTTPWithPostgres`: реальные JWT и repositories через полный
  router; два участника → одна пара, третий не видит её, повтор 409, несколько
  страниц, чужой cursor 400, спящий caller 403 на свайп, но видит свои пары.
  Только URL фото — тестовый адаптер; настоящий S3 проверяется отдельно T08.

Они используют `pgtest.NewDatabase` и `PULSE_TEST_DATABASE_URL` изолированного
test-сервера. Нельзя указывать пользовательскую БД или запускать их с пустыми
SQL как доказательство завершения задач. Факт появления harness не закрывает T13.
