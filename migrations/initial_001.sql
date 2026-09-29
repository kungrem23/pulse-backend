BEGIN;

CREATE TABLE admins (
    uid             BIGSERIAL PRIMARY KEY,
    username        TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE users (
    uid             BIGSERIAL PRIMARY KEY,
    email           TEXT NOT NULL UNIQUE,
    password_hash   TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE profiles (
    uid             BIGINT PRIMARY KEY
                    REFERENCES users(uid) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT,
    age             INT CHECK (age IS NULL OR age BETWEEN 18 AND 120),
    gender          TEXT CHECK (gender IN ('male','female','other')),
    preferences     TEXT,
    is_sleep        BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE photos (
    uid         BIGSERIAL PRIMARY KEY,
    user_uid    BIGINT NOT NULL
                REFERENCES profiles(uid) ON DELETE CASCADE,
    s3_id       TEXT,
    position    INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_photos_user_position UNIQUE (user_uid, position)
);

CREATE INDEX idx_photos_user_uid ON photos(user_uid);

CREATE TABLE preferences (
    uid        BIGINT PRIMARY KEY
               REFERENCES profiles(uid) ON DELETE CASCADE,
    min_age    INT CHECK (min_age IS NULL OR min_age >= 18),
    max_age    INT,
    sex        TEXT CHECK (sex IN ('male','female','other','any')),
    CONSTRAINT chk_preferences_age_range
        CHECK (min_age IS NULL OR max_age IS NULL OR min_age <= max_age)
);

CREATE TABLE likes (
    uid             BIGSERIAL PRIMARY KEY,
    sender_uid      BIGINT NOT NULL
                    REFERENCES profiles(uid) ON DELETE CASCADE,
    receiver_uid    BIGINT NOT NULL
                    REFERENCES profiles(uid) ON DELETE CASCADE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_likes_not_self CHECK (sender_uid <> receiver_uid),
    CONSTRAINT uq_likes_pair      UNIQUE (sender_uid, receiver_uid)
);

CREATE INDEX idx_likes_receiver_uid ON likes(receiver_uid);
CREATE INDEX idx_likes_sender_uid   ON likes(sender_uid);

CREATE TABLE matches (
    uid           BIGSERIAL PRIMARY KEY,
    person1_uid   BIGINT NOT NULL
                  REFERENCES profiles(uid) ON DELETE CASCADE,
    person2_uid   BIGINT NOT NULL
                  REFERENCES profiles(uid) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_matches_not_self CHECK (person1_uid <> person2_uid),
    CONSTRAINT chk_matches_order    CHECK (person1_uid < person2_uid),
    CONSTRAINT uq_matches_pair      UNIQUE (person1_uid, person2_uid)
);

CREATE INDEX idx_matches_person1_uid ON matches(person1_uid);
CREATE INDEX idx_matches_person2_uid ON matches(person2_uid);

CREATE TABLE messages (
    uid             BIGSERIAL PRIMARY KEY,
    sender_uid      BIGINT NOT NULL
                    REFERENCES profiles(uid) ON DELETE CASCADE,
    receiver_uid    BIGINT NOT NULL
                    REFERENCES profiles(uid) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    message         TEXT NOT NULL,
    replied_uid     BIGINT
                    REFERENCES messages(uid) ON DELETE SET NULL,
    read_at         TIMESTAMPTZ,
    CONSTRAINT chk_messages_not_self CHECK (sender_uid <> receiver_uid)
);

CREATE INDEX idx_messages_receiver_created
    ON messages(receiver_uid, created_at DESC);
CREATE INDEX idx_messages_sender_created
    ON messages(sender_uid, created_at DESC);
CREATE INDEX idx_messages_replied_uid ON messages(replied_uid);

CREATE TABLE reports (
    uid             BIGSERIAL PRIMARY KEY,
    sender_uid      BIGINT NOT NULL
                    REFERENCES profiles(uid) ON DELETE CASCADE,
    receiver_uid    BIGINT NOT NULL
                    REFERENCES profiles(uid) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    reason          TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'new'
                    CHECK (status IN ('new','reviewing','resolved','rejected')),
    processed_by    BIGINT REFERENCES admins(uid) ON DELETE SET NULL,
    processed_at    TIMESTAMPTZ,
    CONSTRAINT chk_reports_not_self CHECK (sender_uid <> receiver_uid)
);

CREATE INDEX idx_reports_receiver_uid ON reports(receiver_uid);
CREATE INDEX idx_reports_sender_uid   ON reports(sender_uid);
CREATE INDEX idx_reports_status       ON reports(status)
    WHERE status IN ('new','reviewing');

CREATE TABLE schema_migrations (
    version     TEXT PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO schema_migrations(version) VALUES ('initial_001');

COMMIT;