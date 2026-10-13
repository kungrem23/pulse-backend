# SQL-запросы Pulse MVP

Все 38 заполненных SQL-плейсхолдеров: 30 запросов репозиториев и 8 запросов
тестовых данных. Ниже приведены полные тексты, назначение и расположение.
Исполняемые константы остаются в Go-файлах: этот Markdown — справочник,
сервер не читает SQL из документации. При изменении SQL обновляйте обе копии.

## Схема и параметры

Запросы рассчитаны на [decisions_002.sql](../migrations/decisions_002.sql),
применяемую после [initial_001.sql](../migrations/initial_001.sql) на пустой БД.
`profiles.city` уже объявлен как обязательный `TEXT` длиной 1–100 символов;
`refresh_sessions` уже содержит UUID, владельца, уникальный хэш refresh,
срок действия, время отзыва и ссылку `replaced_by`. Новая миграция для этих
двух изменений не нужна. Непустая legacy-база требует отдельного плана переноса.

`$1`, `$2` и далее — параметры pgx, значения никогда не конкатенируются в SQL.
Порядок аргументов и результатов указан в комментариях непосредственно над
константами в Go; `Scan` читает столбцы позиционно. UUID в БД имеют тип `uuid`,
а в Go передаются строками; `::text` используется в результатах, `::uuid` —
в параметрах, `::uuid[]` — в массивах. Ошибки UNIQUE/FK переводятся репозиториями
в доменные ошибки и не раскрываются публичным API.

Ротация refresh и обновление возраста с анкетой атомарны. Для свайпа порядок
блокировок: `users → profiles → preferences → photos`, UUID отсортированы.
Встречные лайки ждут общую блокировку пользователей, затем повторно проверяют
состояние и обратный лайк; один UNIQUE в matches сам по себе был бы недостаточен.
Все блокировки удерживаются до конца транзакции. Известные откаты из-за deadlock
или serialization failure повторяются максимум три раза; неоднозначные ошибки
COMMIT не повторяются автоматически.

Скорость ленты на рабочих объёмах данных и цель 0,8 секунды требуют отдельного
измерения. Текущая схема уже индексирует исходящие оценки, позиции фото и
списки мэтчей; дополнительных индексов без измерения здесь не добавлено.

## user.go

Расположение: [internal/repository/postgres/user.go](../internal/repository/postgres/user.go).

### `createUserQuery`

Создаёт пользователя с нормализованным email, хэшем пароля и возрастом. Возвращает UUID, блокировку и время создания; UNIQUE email предотвращает конкурентные дубликаты.

```sql
INSERT INTO users (email, password_hash, age)
VALUES ($1, $2, $3)
RETURNING id::text, is_blocked, created_at;
```

### `userByEmailQuery`

Находит пользователя для входа. Хэш пароля нужен только сервису аутентификации и не попадает в публичный DTO.

```sql
SELECT id::text, email, password_hash, age, is_blocked, created_at
FROM users WHERE email = $1;
```

### `userByIDQuery`

Читает пользователя по UUID для проверки access JWT, блокировки и обновления сессии.

```sql
SELECT id::text, email, password_hash, age, is_blocked, created_at
FROM users WHERE id = $1::uuid;
```

## refresh_session.go

Расположение: [internal/repository/postgres/refresh_session.go](../internal/repository/postgres/refresh_session.go).

### `createRefreshSessionQuery`

Сохраняет хэш refresh JWT, владельца и срок действия. Возвращает UUID серверной сессии; исходный JWT в таблице не хранится.

```sql
INSERT INTO refresh_sessions (user_id, token_hash, expires_at)
VALUES ($1::uuid, $2, $3)
RETURNING id::text;
```

### `lockActiveRefreshSessionQuery`

Находит действующую сессию данного владельца и блокирует её FOR UPDATE. После конкурирующего отзыва ожидающая ротация не получает строку. MATERIALIZED CTE сначала получает блокировку; внешний SELECT проверяет срок по clock_timestamp() уже после ожидания, поэтому просроченная за это время сессия не обновляется.

```sql
WITH locked_session AS MATERIALIZED (
    SELECT id, revoked_at, expires_at FROM refresh_sessions
    WHERE token_hash = $1 AND user_id = $2::uuid
    FOR UPDATE
)
SELECT id::text FROM locked_session
WHERE revoked_at IS NULL AND expires_at > clock_timestamp();
```

### `markRefreshSessionReplacedQuery`

Отзывает прежнюю сессию и связывает её с новой. Выполняется внутри той же транзакции, что блокировка и создание новой сессии.

```sql
UPDATE refresh_sessions
SET revoked_at = clock_timestamp(), replaced_by = $2::uuid
WHERE id = $1::uuid AND revoked_at IS NULL;
```

### `revokeRefreshSessionQuery`

Отзывает сессию только указанного владельца. Повторный logout ничего не изменяет: ноль обновлённых строк допустим.

```sql
UPDATE refresh_sessions SET revoked_at = clock_timestamp()
WHERE user_id = $1::uuid AND token_hash = $2 AND revoked_at IS NULL;
```

## profile.go

Расположение: [internal/repository/postgres/profile.go](../internal/repository/postgres/profile.go).

### `updateUserAgeQuery`

Обновляет единственное хранилище возраста — users.age — и время изменения. Выполняется в одной транзакции с сохранением анкеты; отсутствие пользователя определяется по числу изменённых строк.

```sql
UPDATE users SET age = $2, updated_at = now()
WHERE id = $1::uuid;
```

### `upsertProfileQuery`

Создаёт либо редактирует имя, bio, пол и обязательный город. ON CONFLICT защищает первое сохранение от гонок. Существующий is_sleeping не перезаписывается; возвращается сохранённый флаг сна.

```sql
INSERT INTO profiles (user_id, name, bio, sex, city)
VALUES ($1::uuid, $2, $3, $4, $5)
ON CONFLICT (user_id) DO UPDATE
SET name = EXCLUDED.name, bio = EXCLUDED.bio, sex = EXCLUDED.sex,
    city = EXCLUDED.city, updated_at = now()
RETURNING is_sleeping;
```

### `profileByUserIDQuery`

Читает анкету, включая город и сон. Возраст берётся из users через JOIN, без второго столбца age в profiles.

```sql
SELECT p.user_id::text, p.name, p.bio, u.age, p.sex, p.city, p.is_sleeping
FROM profiles p JOIN users u ON u.id = p.user_id
WHERE p.user_id = $1::uuid;
```

### `preferencesByUserIDQuery`

Читает диапазон возрастов и единственное предпочтение пола. До заполнения предпочтений строка отсутствует; выдуманные значения по умолчанию не создаются.

```sql
SELECT user_id::text, min_age, max_age, sex
FROM preferences WHERE user_id = $1::uuid;
```

### `upsertPreferencesQuery`

Создаёт или заменяет предпочтения и возвращает их. FK требует существующую анкету; CHECK проверяет диапазон 18–120, min <= max и male/female/any.

```sql
INSERT INTO preferences (user_id, min_age, max_age, sex)
VALUES ($1::uuid, $2, $3, $4)
ON CONFLICT (user_id) DO UPDATE
SET min_age = EXCLUDED.min_age, max_age = EXCLUDED.max_age, sex = EXCLUDED.sex
RETURNING user_id::text, min_age, max_age, sex;
```

### `setSleepQuery`

Изменяет только сон и updated_at анкеты. RETURNING позволяет вернуть фактическое состояние; отсутствующая анкета даёт ноль строк.

```sql
UPDATE profiles SET is_sleeping = $2, updated_at = now()
WHERE user_id = $1::uuid
RETURNING is_sleeping;
```

## photo.go

Расположение: [internal/repository/postgres/photo.go](../internal/repository/postgres/photo.go).

### `createPhotoQuery`

Создаёт метаданные фото. Позиции 1–5 и UNIQUE(user_id, position) обеспечивают предел пяти фотографий и конфликт конкурентных загрузок. S3-объект создаётся отдельно; при SQL-ошибке application удаляет его компенсацией.

```sql
INSERT INTO photos (id, user_id, object_key, position)
VALUES ($1::uuid, $2::uuid, $3, $4);
```

### `photoByIDForUserQuery`

Ищет фото одновременно по UUID и владельцу. Чужое и отсутствующее фото одинаково дают ноль строк.

```sql
SELECT id::text, user_id::text, object_key, position FROM photos
WHERE id = $1::uuid AND user_id = $2::uuid;
```

### `photosByUserIDQuery`

Возвращает все фото только данного пользователя в порядке позиций. В БД читаются приватные ключи; URL подписывает S3-адаптер.

```sql
SELECT id::text, user_id::text, object_key, position FROM photos
WHERE user_id = $1::uuid ORDER BY position ASC;
```

### `deletePhotoQuery`

Удаляет метаданные только собственного фото. S3 удаляется application-слоем до этого запроса; повтор после SQL-ошибки завершает очистку. Анкета и предпочтения сохраняются.

```sql
DELETE FROM photos WHERE id = $1::uuid AND user_id = $2::uuid;
```

## search_state.go

Расположение: [internal/repository/postgres/search_state.go](../internal/repository/postgres/search_state.go).

### `searchStateByUserIDQuery`

Читает блокировку, завершённость анкеты/предпочтений, число фото и сон. LEFT JOIN сохраняет пользователя до онбординга; COALESCE исключает NULL. Запрос используется и в snapshot ленты, и повторно внутри транзакции свайпа.

```sql
SELECT u.is_blocked,
       COALESCE(char_length(btrim(p.name)) BETWEEN 1 AND 100
           AND u.age BETWEEN 18 AND 120 AND p.sex IN ('male', 'female')
           AND char_length(btrim(p.city)) BETWEEN 1 AND 100, false),
       COALESCE(pr.min_age BETWEEN 18 AND 120 AND pr.max_age BETWEEN 18 AND 120
           AND pr.min_age <= pr.max_age AND pr.sex IN ('male', 'female', 'any'), false),
       (SELECT count(*)::int FROM photos ph WHERE ph.user_id = u.id),
       COALESCE(p.is_sleeping, false)
FROM users u
LEFT JOIN profiles p ON p.user_id = u.id
LEFT JOIN preferences pr ON pr.user_id = p.user_id
WHERE u.id = $1::uuid;
```

## discovery.go

Расположение: [internal/repository/postgres/discovery.go](../internal/repository/postgres/discovery.go).

### `discoveryCandidatesQuery`

Отбирает карточки по всем четырём взаимным условиям возраста и пола. Исключает себя, блокировку, сон, отсутствие анкеты/предпочтений/фото, все исходящие оценки и все пары независимо от статуса. Входящий лайк сам по себе не исключает кандидата. Город не фильтруется. Схема CHECK гарантирует допустимые поля анкеты. Keyset по (created_at, id) применяется до LIMIT; сервис передаёт limit + 1 для определения следующей страницы.

```sql
SELECT u.id::text, p.name, p.bio, u.age, p.sex, p.city, u.created_at
FROM users me
JOIN profiles mp ON mp.user_id = me.id
JOIN preferences mine ON mine.user_id = me.id
JOIN users u ON u.id <> me.id
JOIN profiles p ON p.user_id = u.id
JOIN preferences theirs ON theirs.user_id = u.id
WHERE me.id = $1::uuid
  AND NOT me.is_blocked AND NOT mp.is_sleeping
  AND NOT u.is_blocked AND NOT p.is_sleeping
  AND u.age BETWEEN mine.min_age AND mine.max_age
  AND (mine.sex = 'any' OR mine.sex = p.sex)
  AND me.age BETWEEN theirs.min_age AND theirs.max_age
  AND (theirs.sex = 'any' OR theirs.sex = mp.sex)
  AND EXISTS (SELECT 1 FROM photos ph WHERE ph.user_id = u.id)
  AND NOT EXISTS (SELECT 1 FROM swipes s WHERE s.from_user_id = me.id AND s.to_user_id = u.id)
  AND NOT EXISTS (SELECT 1 FROM matches m
      WHERE m.person1_id = LEAST(me.id, u.id) AND m.person2_id = GREATEST(me.id, u.id))
  AND ($2::timestamptz IS NULL OR (u.created_at, u.id) < ($2::timestamptz, $3::uuid))
ORDER BY u.created_at DESC, u.id DESC
LIMIT $4;
```

### `discoveryPhotosQuery`

Одним запросом получает фотографии только UUID кандидатов текущей страницы. Выполняется в одной read-only REPEATABLE READ транзакции с отбором карточек; нет отдельного SQL-запроса на каждую карточку.

```sql
SELECT id::text, user_id::text, object_key, position
FROM photos WHERE user_id = ANY($1::uuid[])
ORDER BY user_id, position ASC;
```

## swipe.go

Расположение: [internal/repository/postgres/swipe.go](../internal/repository/postgres/swipe.go).

### `lockSwipeUsersQuery`

Блокирует обоих пользователей в порядке UUID. Встречные и пересекающиеся пары сериализуются; изменение возраста/блокировки не проходит одновременно с проверкой и записью свайпа.

```sql
SELECT id FROM users WHERE id = ANY($1::uuid[])
ORDER BY id FOR UPDATE;
```

### `lockSwipeProfilesQuery`

После users блокирует анкеты в том же порядке. Защищает пол/город/сон, а также взаимодействие с внешними ключами фотографий и предпочтений.

```sql
SELECT user_id FROM profiles WHERE user_id = ANY($1::uuid[])
ORDER BY user_id FOR UPDATE;
```

### `lockSwipePreferencesQuery`

Блокирует предпочтения обоих пользователей. Свайп проверяет согласованный диапазон/пол, защищённый от одновременного редактирования.

```sql
SELECT user_id FROM preferences WHERE user_id = ANY($1::uuid[])
ORDER BY user_id FOR UPDATE;
```

### `lockSwipePhotosQuery`

Блокирует существующие фотографии в порядке владельца и позиции. Удаление последней фотографии сериализуется с проверкой доступности свайпа.

```sql
SELECT id FROM photos WHERE user_id = ANY($1::uuid[])
ORDER BY user_id, position FOR UPDATE;
```

### `swipeHistoryConflictQuery`

Проверяет прежнюю исходящую оценку или существующий мэтч. UUID пары приводятся к каноническому порядку LEAST/GREATEST; закрытая пара тоже конфликтует.

```sql
SELECT EXISTS (SELECT 1 FROM swipes WHERE from_user_id = $1::uuid AND to_user_id = $2::uuid)
    OR EXISTS (SELECT 1 FROM matches
        WHERE person1_id = LEAST($1::uuid, $2::uuid) AND person2_id = GREATEST($1::uuid, $2::uuid));
```

### `swipeMutualPreferencesQuery`

Проверяет возраст B и пол B относительно предпочтений A, а также возраст A и пол A относительно предпочтений B. any допускает любой из двух полов. При отсутствующих данных EXISTS возвращает false.

```sql
SELECT EXISTS (
    SELECT 1 FROM users a
    JOIN profiles ap ON ap.user_id = a.id
    JOIN preferences ar ON ar.user_id = a.id
    JOIN users b ON b.id = $2::uuid
    JOIN profiles bp ON bp.user_id = b.id
    JOIN preferences br ON br.user_id = b.id
    WHERE a.id = $1::uuid
      AND b.age BETWEEN ar.min_age AND ar.max_age
      AND (ar.sex = 'any' OR ar.sex = bp.sex)
      AND a.age BETWEEN br.min_age AND br.max_age
      AND (br.sex = 'any' OR br.sex = ap.sex)
);
```

### `insertSwipeQuery`

Один раз сохраняет направленный лайк или пропуск. Здесь намеренно нет UPSERT: предыдущую оценку и историю мэтча нельзя переписать.

```sql
INSERT INTO swipes (from_user_id, to_user_id, is_like)
VALUES ($1::uuid, $2::uuid, $3);
```

### `reciprocalLikeQuery`

Проверяет именно обратный лайк B → A. Обратный пропуск не создаёт мэтч.

```sql
SELECT EXISTS (SELECT 1 FROM swipes
    WHERE from_user_id = $2::uuid AND to_user_id = $1::uuid AND is_like);
```

### `insertSwipeMatchQuery`

Создаёт активный мэтч упорядоченной пары и возвращает его. UNIQUE пары действует навсегда. Свайп, проверки, блокировки и создание мэтча находятся в одной транзакции READ COMMITTED; успешный ответ выдаётся только после COMMIT.

```sql
INSERT INTO matches (person1_id, person2_id)
VALUES ($1::uuid, $2::uuid)
RETURNING id::text, person1_id::text, person2_id::text, status, created_at;
```

## match.go

Расположение: [internal/repository/postgres/match.go](../internal/repository/postgres/match.go).

### `matchesByUserIDQuery`

Возвращает только пары текущего пользователя и данные другого участника. Сон/блокировка партнёра и закрытый статус не удаляют историю. LEFT JOIN LATERAL выбирает одно фото с минимальной позицией; при отсутствии фото поля NULL. Keyset ограничивает пары, а не строки фотографий. Чат не требуется.

```sql
SELECT m.id::text, m.person1_id::text, m.person2_id::text, m.status, m.created_at,
       p.user_id::text, p.name, p.bio, u.age, p.sex, p.city, p.is_sleeping,
       ph.id::text, ph.user_id::text, ph.object_key, ph.position
FROM matches m
JOIN profiles p ON p.user_id = CASE WHEN m.person1_id = $1::uuid THEN m.person2_id ELSE m.person1_id END
JOIN users u ON u.id = p.user_id
LEFT JOIN LATERAL (
    SELECT id, user_id, object_key, position FROM photos
    WHERE user_id = p.user_id ORDER BY position ASC LIMIT 1
) ph ON true
WHERE (m.person1_id = $1::uuid OR m.person2_id = $1::uuid)
  AND ($2::timestamptz IS NULL OR (m.created_at, m.id) < ($2::timestamptz, $3::uuid))
ORDER BY m.created_at DESC, m.id DESC
LIMIT $4;
```

## discovery_test.go

Расположение: [internal/repository/postgres/discovery_test.go](../internal/repository/postgres/discovery_test.go).

Это SQL только для изолированных тестовых БД `pgtest`, не операции публичного API.

### `discoveryTestBlockUserQuery`

Блокирует тестового кандидата для проверки исключений из ленты.

```sql
UPDATE users SET is_blocked = true, updated_at = now() WHERE id = $1::uuid;
```

### `discoveryTestSwipeQuery`

Создаёт тестовый лайк либо пропуск для проверки исходящих/входящих оценок.

```sql
INSERT INTO swipes (from_user_id, to_user_id, is_like) VALUES ($1::uuid, $2::uuid, $3);
```

### `discoveryTestMatchQuery`

Создаёт активную или закрытую тестовую пару; closed_at согласован со статусом.

```sql
INSERT INTO matches (person1_id, person2_id, status, closed_at)
VALUES ($1::uuid, $2::uuid, $3, CASE WHEN $3 = 'closed' THEN now() ELSE NULL END);
```

### `discoveryTestCreatedAtQuery`

Задаёт одинаковое время создания выбранным пользователям, чтобы проверить UUID как второй ключ пагинации.

```sql
UPDATE users SET created_at = $1::timestamptz WHERE id = ANY($2::uuid[]);
```

## matching_test.go

Расположение: [internal/repository/postgres/matching_test.go](../internal/repository/postgres/matching_test.go).

Это SQL только для изолированных тестовых БД `pgtest`, не операции публичного API.

### `matchingTestBlockUserQuery`

Блокирует тестового участника для проверки запрета свайпа.

```sql
UPDATE users SET is_blocked = true, updated_at = now() WHERE id = $1::uuid;
```

### `matchingTestSwipeQuery`

Читает сохранённую оценку пары для проверки записи и отсутствия её перезаписывания.

```sql
SELECT is_like FROM swipes WHERE from_user_id = $1::uuid AND to_user_id = $2::uuid;
```

### `matchingTestPairMatchesQuery`

Читает все мэтчи одной тестовой пары, чтобы проверить единственность и сохранность истории.

```sql
SELECT id::text, person1_id::text, person2_id::text, status, created_at
FROM matches WHERE person1_id = $1::uuid AND person2_id = $2::uuid;
```

### `matchingTestCreateMatchQuery`

Создаёт тестовый мэтч с заданными временем и статусом для проверки сортировки, границ страниц и закрытых пар.

```sql
INSERT INTO matches (person1_id, person2_id, created_at, status, closed_at)
VALUES ($1::uuid, $2::uuid, $3::timestamptz, $4,
    CASE WHEN $4 = 'closed' THEN $3::timestamptz ELSE NULL END)
RETURNING id::text;
```

## Проверка

Для PostgreSQL-тестов задайте `PULSE_TEST_DATABASE_URL` отдельного сервера
с правом CREATE DATABASE. pgtest создаёт уникальную БД, применяет обе миграции
и удаляет её после теста. БД из URL не изменяется. Для tagged S3-тестов также
нужны `S3_ENDPOINT`, `S3_PUBLIC_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`,
`S3_SECRET_KEY` отдельного MinIO с созданным bucket. Без настроек соответствующие
тесты пропускаются. Не используйте пользовательские хранилища.

```sh
go test ./...
go vet ./...
go test -count=1 -race -tags=integration ./...
git diff --check
```

Полный tagged/race прогон с изолированными PostgreSQL 18 и MinIO проходит,
включая SQL/HTTP/S3, concurrent refresh/фото/встречные лайки, keyset и фильтры,
а также общий `TestMVPWithPostgresAndS3`. Отдельный тест подтверждает запрет
ротации refresh, истёкшего во время ожидания блокировки. Производительность
и frontend QA этим прогоном не измеряются.
