-- Registration needs only email/password; age is supplied when creating a profile.
-- Apply once after decisions_002. Existing ages and all other data are preserved.
BEGIN;

LOCK TABLE schema_migrations IN EXCLUSIVE MODE;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 'decisions_002') THEN
        RAISE EXCEPTION 'decisions_002 must be applied first';
    END IF;
    IF EXISTS (SELECT 1 FROM schema_migrations WHERE version = 'profile_age_003') THEN
        RAISE EXCEPTION 'profile_age_003 is already applied';
    END IF;
END
$$;

-- Keep the CHECK (age BETWEEN 18 AND 120): NULL means unfinished onboarding,
-- never an invented age. Application validation requires age on profile saves.
ALTER TABLE users ALTER COLUMN age DROP NOT NULL;

INSERT INTO schema_migrations(version) VALUES ('profile_age_003');

COMMIT;
