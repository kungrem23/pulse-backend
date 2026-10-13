# T09: лента кандидатов

Актуальное состояние 2026-10-13: SQL заполнен и проверен на изолированных
PostgreSQL/MinIO полным `go test -count=1 -race -tags=integration ./...`.
Операции MVP имеют статус `implemented`. Все запросы с пояснениями:
[SQL_QUERIES.md](SQL_QUERIES.md). Новый `TestMVPWithPostgresAndS3` проверяет
общий HTTP-сценарий на обоих реальных хранилищах.

Ниже сохранён **исторический снимок этапа до заполнения SQL**. Его формулировки
«плейсхолдер», «SQL ожидается» и `partial` относятся к прежнему этапу;
текущая карта готовности — [AI/STATUS.md](../AI/STATUS.md).

## Исторический этап

Go-часть подготовлена и подключена в `cmd/api`. `GET /api/v1/feed` вызывает
реальный discovery service, PostgreSQL adapters и S3 URL adapter. По поручению
владельца SQL оставлен плейсхолдерами; фильтрация, работа с реальным PostgreSQL
и оценка запросов/индексов ожидают его заполнения. OpenAPI имеет статус `partial`.

## HTTP и application

UserID берётся из проверенного user access JWT. Лента принимает только `limit`
и `cursor`; передать `user_id`, фильтр города или повторный параметр нельзя.
Limit 1–100, default 20. Пустой/невалидный cursor, неверный limit или неизвестный
параметр дают 400. Cursor имеет согласованный формат из `internal/app/pagination`
и привязан к разделу feed и действующему пользователю.

Сервис проверяет `SearchState.CheckEligible`: незавершённые profile/preferences,
отсутствие фото, sleep или block дают 403. Отсутствующий действующий пользователь
даёт 401. Repository повторно проверяет eligibility внутри снимка чтения;
изменение состояния после предварительной проверки не обходится прямым вызовом.

Ответ содержит `{items, next_cursor?}`, карточка — user_id/name/bio/age/sex/city
и фото `{id, url, position}`. URL подписываются при каждом запросе; object key
остаётся внутренним. При исчерпании `items: []`, `next_cursor` отсутствует,
история свайпов не сбрасывается. Ошибки БД/подписи URL дают общий 500.

## SQL-плейсхолдеры

| Файл и константа | Параметры | Результат |
| --- | --- | --- |
| `search_state.go`: `searchStateByUserIDQuery` | user UUID | is_blocked, has_completed_profile, has_valid_preferences, photo_count, is_sleeping; без NULL |
| `discovery.go`: `discoveryCandidatesQuery` | caller UUID, nullable after time, nullable after user UUID, limit + 1 | user_id::text, name, bio, users.age, sex, city, users.created_at; одна строка на кандидата |
| `discovery.go`: `discoveryPhotosQuery` | UUID-массив выданных кандидатов | id::text, user_id::text, object_key, position; порядок user_id, position ASC |

SearchState query начинается с users и сохраняет строку существующего
пользователя даже без анкеты/preferences/фото. Завершённость проверяется по
согласованным полям и диапазонам. Только отсутствующий users даёт ErrNotFound;
отсутствие онбординга возвращает неполный SearchState.

Candidate query обязан применить до LIMIT все четыре условия:

1. Возраст B входит в диапазон A.
2. Пол B соответствует предпочтению A или A выбрал any.
3. Возраст A входит в диапазон B.
4. Пол A соответствует предпочтению B или B выбрал any.

Исключаются self, block/sleep, неполная анкета/preferences, отсутствие фото,
любой исходящий свайп (лайк или пропуск), любой active/closed мэтч в любой
позиции пары. Один входящий лайк не исключает кандидата. Город — поле карточки,
а не фильтр. Код приложения не подменяет эти условия фильтрацией после LIMIT.

## Снимок, страницы и фото

`DiscoveryRepository.Feed` использует read-only REPEATABLE READ: состояние
действующего пользователя, кандидаты и фото читаются в одном снимке. Например,
удаление последнего фото между candidate query и photo query не создаёт
карточку без фотографий в одном ответе. Это снимок на время чтения, а не
блокировка последующих действий пользователя до окончания HTTP-запроса.

Сортировка: `users.created_at DESC, users.id DESC`, граница After исключительная
по обоим полям. При пустом After оба nullable аргумента — NULL. SQL получает
limit + 1; Go убирает дополнительную строку и выставляет HasMore. Следующий
cursor строится по последней выданной карточке, а не по строке lookahead.

Фото выданной страницы загружаются одним запросом по UUID-массиву, а не отдельно
для каждого пользователя. Lookahead-кандидату фото не загружаются. Repository
проверяет, что фото принадлежат выбранной странице, позиции уникальны и упорядочены,
а у каждого кандидата есть фото. Число SQL-вызовов не растёт с размером страницы:
одна предварительная проверка состояния в app; внутри snapshot — state,
candidate query и один photo query (последний пропускается для пустой страницы).

Миграции/индексы не изменялись. После заполнения SQL требуется проверить план
запроса и индексы на представительном наборе, а также измерить работу ленты.
Показатели производительности сейчас не заявлены выполненными.

## Тесты

Unit/HTTP/router-app проверки уже выполняются без БД: права, query parsing,
cursor другого пользователя/раздела, eligibility и повторная проверка repository,
URL-подпись, пустой JSON-массив, следующий cursor и ошибки без утечки данных.
Router/app-тест связывает T08 и T09: удаление последнего собственного фото
приводит к 403 при следующем запросе ленты. Test adapters не доказывают SQL-фильтры.

`internal/repository/postgres/discovery_test.go` подготовлен для отдельного
PostgreSQL через `PULSE_TEST_DATABASE_URL`. Он покрывает все четыре взаимных
условия, any, 18/120, разные города, self/sleep/block/отсутствие онбординга,
фото, исходящие лайки/пропуски, активные/закрытые мэтчи, входящий лайк,
равные timestamps и UUID tie-breaker, исчерпание и повторную проверку состояния.

В этом тесте тоже намеренно оставлены четыре SQL-плейсхолдера для fixtures:

- `discoveryTestBlockUserQuery`: блокировка одного тестового users ($1 UUID).
- `discoveryTestSwipeQuery`: вставка тестового свайпа ($1 from, $2 to, $3 is_like).
- `discoveryTestMatchQuery`: вставка упорядоченной пары ($1/$2 UUID, $3 status);
  closed_at должен соответствовать active/closed.
- `discoveryTestCreatedAtQuery`: одинаковый users.created_at для тестовых
  кандидатов ($1 timestamp, $2 UUID-массив).

Эти запросы применяются исключительно в уникальной disposable БД из pgtest.
Нужно заполнить также прежние User/Profile/Photo SQL. При отсутствии env тесты
пропускаются, при настроенном env и пустом SQL должны падать.

```sh
go test ./...
go vet ./...
go test -race ./internal/app/discovery ./internal/transport/http/...
go test -count=1 ./internal/repository/postgres -run 'TestDiscovery|TestSearchState' -v
git diff --check
```

Свайпы/список мэтчей, чат и закрытие мэтча этой задачей не реализуются; общие
интерфейсы и форматы cursor из общего коммита сохранены.
