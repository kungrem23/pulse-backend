-- Canonical MVP schema following the owner's decisions dated 2026-10-12.
-- Apply after initial_001, ONLY to an empty application database.
-- Existing records require a separately reviewed data migration; never invent
-- ages or cities or discard user data to get past this guard.
BEGIN;

LOCK TABLE admins, users, profiles, photos, preferences, likes, matches,
    messages, reports, schema_migrations IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 'initial_001') THEN
        RAISE EXCEPTION 'initial_001 must be applied first';
    END IF;
    IF EXISTS (SELECT 1 FROM schema_migrations WHERE version = 'decisions_002') THEN
        RAISE EXCEPTION 'decisions_002 is already applied';
    END IF;
    IF EXISTS (SELECT 1 FROM users)
        OR EXISTS (SELECT 1 FROM admins)
        OR EXISTS (SELECT 1 FROM profiles)
        OR EXISTS (SELECT 1 FROM photos)
        OR EXISTS (SELECT 1 FROM preferences)
        OR EXISTS (SELECT 1 FROM likes)
        OR EXISTS (SELECT 1 FROM matches)
        OR EXISTS (SELECT 1 FROM messages)
        OR EXISTS (SELECT 1 FROM reports) THEN
        RAISE EXCEPTION 'Legacy data exists: a reviewed data migration is required; no data has been changed';
    END IF;
END
$$;

-- All nine tables were locked and checked to be empty. No CASCADE is used:
-- unexpected external dependencies make the whole transaction fail safely.
DROP TABLE reports, messages, matches, likes, preferences, photos, profiles,
    users, admins;

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email           TEXT NOT NULL UNIQUE
                    CHECK (char_length(email) BETWEEN 1 AND 254
                        AND email = lower(btrim(email))),
    password_hash   TEXT NOT NULL,
    age             SMALLINT NOT NULL CHECK (age BETWEEN 18 AND 120),
    is_blocked      BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE profiles (
    user_id         UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    name            TEXT NOT NULL CHECK (char_length(btrim(name)) BETWEEN 1 AND 100),
    bio             TEXT NOT NULL DEFAULT '' CHECK (char_length(bio) <= 500),
    sex             TEXT NOT NULL CHECK (sex IN ('male', 'female')),
    city            TEXT NOT NULL CHECK (char_length(btrim(city)) BETWEEN 1 AND 100),
    is_sleeping     BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Age belongs to users. Profile reads join users.age; profile updates that
-- change age update both tables in one application transaction.
CREATE TABLE preferences (
    user_id         UUID PRIMARY KEY REFERENCES profiles(user_id) ON DELETE CASCADE,
    min_age         SMALLINT NOT NULL CHECK (min_age BETWEEN 18 AND 120),
    max_age         SMALLINT NOT NULL CHECK (max_age BETWEEN 18 AND 120),
    sex             TEXT NOT NULL CHECK (sex IN ('male', 'female', 'any')),
    CHECK (min_age <= max_age)
);

CREATE TABLE photos (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
    object_key      TEXT NOT NULL UNIQUE CHECK (char_length(object_key) > 0),
    position        SMALLINT NOT NULL CHECK (position BETWEEN 1 AND 5),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, position)
);

-- Exactly five available unique positions enforce at most five photos even
-- for concurrent uploads. Content validation and the 10 MiB limit are in app/media.
CREATE TABLE swipes (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    from_user_id    UUID NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
    to_user_id      UUID NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
    is_like         BOOLEAN NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (from_user_id <> to_user_id),
    UNIQUE (from_user_id, to_user_id)
);

CREATE INDEX idx_swipes_to_user ON swipes(to_user_id, from_user_id) WHERE is_like;

CREATE TABLE matches (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    person1_id      UUID NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
    person2_id      UUID NOT NULL REFERENCES profiles(user_id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'closed')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at       TIMESTAMPTZ,
    CHECK (person1_id < person2_id),
    CHECK ((status = 'active' AND closed_at IS NULL)
        OR (status = 'closed' AND closed_at IS NOT NULL)),
    UNIQUE (person1_id, person2_id)
);

CREATE INDEX idx_matches_person1_created ON matches(person1_id, created_at DESC, id DESC);
CREATE INDEX idx_matches_person2_created ON matches(person2_id, created_at DESC, id DESC);

-- MVP user sessions only. Admin sessions belong to a later reviewed migration.
-- Store a hash of the refresh credential, never the raw JWT.
CREATE TABLE refresh_sessions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash          TEXT NOT NULL UNIQUE,
    expires_at          TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at          TIMESTAMPTZ,
    replaced_by         UUID REFERENCES refresh_sessions(id),
    CHECK (expires_at > created_at),
    CHECK (replaced_by IS NULL OR replaced_by <> id)
);

CREATE INDEX idx_refresh_sessions_user ON refresh_sessions(user_id);
CREATE INDEX idx_refresh_sessions_expires ON refresh_sessions(expires_at);

-- Conversation/message, moderation/admin, and notification schemas will be
-- introduced with their respective releases. Their current API is a target contract.
INSERT INTO schema_migrations(version) VALUES ('decisions_002');

COMMIT;
