\set ON_ERROR_STOP on

-- This script is executed only against the isolated Compose test database.

DO $verify$
DECLARE
    generated_id UUID;
BEGIN
    IF (SELECT count(*) FROM schema_migrations
        WHERE version IN ('initial_001', 'decisions_002', 'profile_age_003')) <> 3 THEN
        RAISE EXCEPTION 'expected all three schema migrations';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'profiles'
          AND column_name = 'age'
    ) THEN
        RAISE EXCEPTION 'profiles.age must not exist';
    END IF;

    INSERT INTO users(email, password_hash)
    VALUES ('generated@example.test', 'hash')
    RETURNING id INTO generated_id;
    IF generated_id IS NULL THEN
        RAISE EXCEPTION 'users.id UUID default did not generate a value';
    END IF;
    DELETE FROM users WHERE id = generated_id;

    BEGIN
        INSERT INTO users(email, password_hash, age)
        VALUES ('underage@example.test', 'hash', 17);
        RAISE EXCEPTION 'users.age accepted 17';
    EXCEPTION WHEN check_violation THEN
        NULL;
    END;

    BEGIN
        INSERT INTO users(email, password_hash, age)
        VALUES (' Upper@example.test ', 'hash', 18);
        RAISE EXCEPTION 'users.email accepted a non-normalized value';
    EXCEPTION WHEN check_violation THEN
        NULL;
    END;

    INSERT INTO users(id, email, password_hash, age)
    VALUES
        ('11111111-1111-4111-8111-111111111111', 'one@example.test', 'hash', 18),
        ('22222222-2222-4222-8222-222222222222', 'two@example.test', 'hash', 120);

    INSERT INTO profiles(user_id, name, bio, sex, city)
    VALUES
        ('11111111-1111-4111-8111-111111111111', 'One', '', 'male', 'Moscow'),
        ('22222222-2222-4222-8222-222222222222', 'Two', 'bio', 'female', 'Kazan');

    BEGIN
        INSERT INTO profiles(user_id, name, sex, city)
        VALUES ('11111111-1111-4111-8111-111111111111', 'Duplicate', 'male', '   ');
        RAISE EXCEPTION 'profiles.city accepted whitespace';
    EXCEPTION
        WHEN check_violation OR unique_violation THEN
            NULL;
    END;

    INSERT INTO preferences(user_id, min_age, max_age, sex)
    VALUES
        ('11111111-1111-4111-8111-111111111111', 18, 120, 'any'),
        ('22222222-2222-4222-8222-222222222222', 18, 120, 'male');

    BEGIN
        UPDATE preferences SET min_age = 30, max_age = 20
        WHERE user_id = '11111111-1111-4111-8111-111111111111';
        RAISE EXCEPTION 'preferences accepted min_age > max_age';
    EXCEPTION WHEN check_violation THEN
        NULL;
    END;

    INSERT INTO photos(user_id, object_key, position)
    SELECT
        '11111111-1111-4111-8111-111111111111',
        'users/one/photo-' || position,
        position
    FROM generate_series(1, 5) AS position;

    BEGIN
        INSERT INTO photos(user_id, object_key, position)
        VALUES ('11111111-1111-4111-8111-111111111111', 'users/one/photo-6', 6);
        RAISE EXCEPTION 'photos accepted a sixth position';
    EXCEPTION WHEN check_violation THEN
        NULL;
    END;

    INSERT INTO swipes(from_user_id, to_user_id, is_like)
    VALUES ('11111111-1111-4111-8111-111111111111',
            '22222222-2222-4222-8222-222222222222', true);

    BEGIN
        INSERT INTO swipes(from_user_id, to_user_id, is_like)
        VALUES ('11111111-1111-4111-8111-111111111111',
                '11111111-1111-4111-8111-111111111111', true);
        RAISE EXCEPTION 'swipes accepted a self-swipe';
    EXCEPTION WHEN check_violation THEN
        NULL;
    END;

    INSERT INTO matches(person1_id, person2_id)
    VALUES ('11111111-1111-4111-8111-111111111111',
            '22222222-2222-4222-8222-222222222222');
    UPDATE matches SET status = 'closed', closed_at = now();

    BEGIN
        INSERT INTO matches(person1_id, person2_id)
        VALUES ('11111111-1111-4111-8111-111111111111',
                '22222222-2222-4222-8222-222222222222');
        RAISE EXCEPTION 'matches accepted a duplicate closed pair';
    EXCEPTION WHEN unique_violation THEN
        NULL;
    END;

    BEGIN
        INSERT INTO matches(person1_id, person2_id)
        VALUES ('22222222-2222-4222-8222-222222222222',
                '11111111-1111-4111-8111-111111111111');
        RAISE EXCEPTION 'matches accepted a non-canonical pair';
    EXCEPTION WHEN check_violation THEN
        NULL;
    END;

    INSERT INTO refresh_sessions(user_id, token_hash, expires_at)
    VALUES ('11111111-1111-4111-8111-111111111111',
            'persistence-marker-token-hash', now() + interval '1 day');
END
$verify$;

SELECT 'schema verification passed' AS result;
