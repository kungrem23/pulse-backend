# Общая база для параллельной разработки MVP

Контракты T08–T11 зафиксированы 2026-10-13. Этот документ хранится в Git,
поэтому оба разработчика получают его вместе с общим коммитом. Порты и типы
описаны в `internal/domain/`; бизнес-операции и SQL ещё предстоит реализовать.

## Разделение работы

| Ветка | Задачи | Владение файлами |
| --- | --- | --- |
| Фото и лента | T08, T09; тесты фото/ленты из T13 | `internal/app/media/`, `internal/app/discovery/`, `internal/transport/http/media/`, `internal/transport/http/discovery/`; фото в `app/profile` и `http/profile` |
| Свайпы и мэтчи | T10, T11, общая часть T12; тесты свайпов/пар из T13 | `internal/app/matching/`, `internal/transport/http/matching/`, `internal/transport/http/middleware/`, `internal/transport/http/httpx/` |

Обе ветки создаются от одного общего коммита. Изменения `internal/domain/`,
`internal/app/pagination/`, `cmd/api/main.go`, `router.go` и OpenAPI согласуются
между разработчиками и объединяются одним интегратором. У каждого сервиса свои
unit/HTTP-тесты; общий сценарий с PostgreSQL/MinIO запускается после объединения.

SQL-запросы выполняются отдельным этапом по зафиксированным портам. Проверки
со stub разрешают работать независимо, но не закрывают функцию без реального
repository, wiring в `cmd/api` и integration-тестов.

## Порты хранения

- `PhotoRepository.ByUserID(ctx, userID)` возвращает фотографии только указанного
  пользователя в порядке `position ASC`; при отсутствии фото — пустой срез.
  `Create`, `ByIDForUser` и `Delete` сохраняют прежние сигнатуры. Лимит/позиции
  защищены конкурентно; lookup/delete включают владельца в условие операции.
- `SearchStateRepository.StateByUserID(ctx, userID)` возвращает общий
  `SearchState`. Для существующего пользователя без анкеты/preferences/фото
  возвращается неполное состояние; `ErrNotFound` — только если нет users.
  `SearchState.CheckEligible()` используется обеими ветками.
- `DiscoveryRepository.Feed(ctx, userID, PageQuery)` возвращает
  `PageResult[DiscoveryCandidate]` с фото, без отдельных запросов на каждую
  карточку. Репозиторий применяет взаимные возрастные/половые фильтры, все
  исключения и повторно проверяет eligibility действующего пользователя.
  Фильтра по городу нет.
- `SwipeRepository.Save(ctx, fromUserID, toUserID, isLike)` сериализует UUID-пару
  и внутри одной транзакции проверяет состояние/фильтры, сохраняет свайп и при
  встречном лайке создаёт мэтч. Возвращает `nil` без мэтча либо `*Match`.
  Повторный исходящий свайп или существующая active/closed пара дают
  `ErrConflict`; история не перезаписывается. Чат не создаётся.
- `MatchRepository.ListByUserID(ctx, userID, PageQuery)` возвращает
  `PageResult[MatchWithPartner]`: только пары участника, данные другого
  пользователя и опциональное первое фото по позиции. Сон, неполный профиль,
  отсутствие фото или закрытие пары не скрывают уже существующий мэтч.

Проверка eligibility в application — предварительная. Для свайпа репозиторий
повторяет проверки внутри транзакции с защитой от конкурентных изменений
состояния анкеты, сна, блокировки, preferences и удаления последнего фото.
Простой вызов `CheckEligible` до `Save` не обеспечивает этот инвариант.
Сервис matching не вызывает discovery service и не ищет кандидата в странице
ленты: обе реализации зависят от domain ports.

## Модели результатов

Repository-проекции `DiscoveryCandidate` и `MatchWithPartner` содержат внутренние
photo object keys. Application преобразует ключи через `PhotoStorage.URL` при
каждом чтении; ссылки могут истекать и не сохраняются в PostgreSQL.

`discovery.Candidate.Photos` использует безопасный `media.Photo` с ID, URL и
позицией. `matching.Service.List` возвращает `Page[matching.MatchSummary]`,
содержащий ID, status, created_at и `PublicUserSummary` партнёра с ID, именем,
возрастом и опциональным photo URL. В MVP `ConversationID` отсутствует. HTTP
использует DTO; доменные записи напрямую в JSON не выдаются.

Будущие конструкторы сервисов должны принимать порты, а не конкретный PostgreSQL
или другой application service. Для discovery нужны DiscoveryRepository,
SearchStateRepository и PhotoStorage; для matching — SwipeRepository,
MatchRepository, SearchStateRepository и PhotoStorage. Их реализация и wiring
относятся к соответствующим веткам и последующему объединению.

## Общая пагинация

Общий код: `internal/app/pagination`. Application получает `domain.PageRequest`,
вызывает `pagination.Parse(request, scope, authenticatedUserID)` и передаёт
репозиторию `domain.PageQuery`.

- `limit`: 1–100, по умолчанию 20. В `PageRequest` ноль обозначает отсутствие
  параметра; HTTP отвергает явно переданный `limit=0`, нецелое значение,
  дублирующий параметр и выход за границы.
- Scope: `pagination.Feed` или `pagination.Matches`. Cursor привязан к scope
  и user ID; использовать его для другого списка/пользователя нельзя.
- Формат v1: base64url без padding от JSON с полями `v`, `scope`, `user_id`,
  `created_at`, `id`. Время — UTC RFC3339Nano, ID — UUID в стандартной форме.
  Максимум 512 символов; неизвестная версия/поля, неверное время/UUID или
  повреждённое кодирование дают `ErrInvalidCursor`.
- Cursor не подписан и не разрешает доступ: репозиторий всегда заново применяет
  user ID, ownership и фильтры независимо от позиции из cursor.
- Лента сортируется по `(users.created_at DESC, users.id DESC)`, пары — по
  `(matches.created_at DESC, matches.id DESC)`. `After` — исключительная граница
  по обоим полям; время изменения профиля не участвует в сортировке.
- Repository возвращает максимум `Limit` элементов; читает ещё один для
  определения `HasMore`. Следующий cursor строится через `pagination.Encode`
  по последнему **выданному** элементу, только если `HasMore == true`.
- Последняя страница не содержит `next_cursor`. В HTTP `items` всегда массив,
  включая `[]`; явно пустой `cursor=` и дублирующий cursor дают 400.

## Общие ошибки

| Ошибка | HTTP для feed/swipe/matches |
| --- | --- |
| `ErrInvalidCursor`, `ErrInvalidLimit` | 400 `bad_request` |
| `ErrProfileIncomplete`, `ErrNoPhotos`, `ErrSleeping`, `ErrBlocked` действующего пользователя | 403 `forbidden` |
| `ErrCandidateUnavailable` (target отсутствует, не готов или не совпадают фильтры) | 404 `not_found` |
| `ErrConflict` (повторный исходящий свайп или существующая пара) | 409 `conflict` |
| `ErrSelfSwipe` | 422 `validation_failed`, field `to_user_id` |

`ErrInvalidUserID` обозначает невалидный UUID на application-границе. HTTP
отвергает невалидный `to_user_id` как 422; действующий UserID поступает из JWT.
Отсутствие действующего пользователя (`ErrNotFound`) переводится в 401.
UUID фото проверяется до операции; отсутствующее/чужое фото даёт одинаковый 404.
В media сохраняются существующие application errors для размера/формата/позиции.

Application/repositories могут оборачивать ошибки; handlers используют
`errors.Is` и общий envelope из `httpx`. Не передавать клиенту SQL/S3 errors.

## Объединение и проверка

Сначала объединяется фото/лента, затем свайпы/мэтчи; интегратор подключает
конкретные repository/services в `cmd/api`, актуализирует OpenAPI и статусы.
Каждая ветка выполняет `gofmt`, `go test ./...`, `go vet ./...` и
`git diff --check`. При наличии SQL запускаются integration-тесты на отдельном
PostgreSQL и MinIO, включая конкурентные загрузки и встречные лайки.

После общего коммита Go-часть T08 реализована и подключена к HTTP/`cmd/api`;
photo SQL оставлен плейсхолдерами. Прогресс и запуск реальных тестов описаны
в [T08_PHOTOS.md](T08_PHOTOS.md). Фото остаются `partial` до SQL/приёмки;
Go-часть ленты также подключена, SQL/фильтры ожидают заполнения и проверки
([T09_DISCOVERY.md](T09_DISCOVERY.md)). Свайпы и список пар ещё не реализованы
сквозным путём. Закрытие мэтча,
чат, модерация, CI и deployment остаются за пределами текущих веток.
