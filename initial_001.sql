

BEGIN;


CREATE TABLE admins (
    uid             BIGSERIAL PRIMARY KEY,
    username        TEXT NOT NULL,
    password_hash   TEXT NOT NULL
);


CREATE TABLE users (
    uid             BIGSERIAL PRIMARY KEY,
    password_hash   TEXT NOT NULL,
    email           TEXT NOT NULL
);


CREATE TABLE profiles (
    uid             BIGSERIAL PRIMARY KEY,
    name            TEXT NOT NULL,
    description     TEXT,
    age             INT,
    gender          TEXT,
    preferences     TEXT,
    is_sleep        BOOLEAN NOT NULL DEFAULT false
);


CREATE TABLE photos (
    uid        BIGSERIAL PRIMARY KEY,
    user_uid   BIGINT NOT NULL REFERENCES profiles(uid),
    s3_id      TEXT,
    position   INT
);


CREATE TABLE preferences (
    uid        BIGSERIAL PRIMARY KEY,
    user_uid   BIGINT NOT NULL REFERENCES profiles(uid),
    min_age    INT,
    max_age    INT,
    sex        TEXT
);


CREATE TABLE likes (
    uid             BIGSERIAL PRIMARY KEY,
    sender_uid      BIGINT REFERENCES profiles(uid),
    receiver_uid    BIGINT REFERENCES profiles(uid),
    created         TIMESTAMPTZ
);


CREATE TABLE matches (
    uid           BIGSERIAL PRIMARY KEY,
    person1_uid   BIGINT REFERENCES profiles(uid),
    person2_uid   BIGINT REFERENCES profiles(uid),
    created_at    TIMESTAMPTZ
);


CREATE TABLE messages (
    uid             BIGSERIAL PRIMARY KEY,
    sender_uid      BIGINT REFERENCES profiles(uid),
    receiver_uid    BIGINT REFERENCES profiles(uid),
    created_at      TIMESTAMPTZ,
    message         TEXT,
    replied_uid     BIGINT REFERENCES messages(uid)
);


CREATE TABLE reports (
    uid             BIGSERIAL PRIMARY KEY,
    sender_uid      BIGINT REFERENCES profiles(uid),
    receiver_uid    BIGINT REFERENCES profiles(uid),
    created_at      TIMESTAMPTZ,
    reason          TEXT,
    is_processed    BOOLEAN NOT NULL DEFAULT false
);


CREATE TABLE schema_migrations (
    version     TEXT PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO schema_migrations(version) VALUES ('initial_001');

COMMIT;