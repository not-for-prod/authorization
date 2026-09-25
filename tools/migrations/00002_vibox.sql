-- +goose Up

-- Canonical vibox schema migration.

-- This baseline is intentionally adoption-safe for databases initialized by
-- the former unversioned schema helper. Future migrations should use strict,
-- incremental DDL instead of repeating this IF NOT EXISTS pattern.
CREATE TABLE IF NOT EXISTS event_topic_partition (
    topic           text    NOT NULL,
    partition       integer NOT NULL,
    last_position   bigint  NOT NULL DEFAULT 0,

    PRIMARY KEY (topic, partition),

    CHECK (topic <> ''),
    CHECK (partition >= 0),
    CHECK (last_position >= 0)
);

CREATE TABLE IF NOT EXISTS event_log (
    topic           text        NOT NULL,
    partition       integer     NOT NULL,
    position        bigint      NOT NULL,
    partition_key   bytea       NOT NULL,
    payload         bytea       NOT NULL,
    trace_carrier   jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL DEFAULT clock_timestamp(),

    PRIMARY KEY (topic, partition, position),

    FOREIGN KEY (topic, partition)
        REFERENCES event_topic_partition (topic, partition),

    CHECK (topic <> ''),
    CHECK (partition >= 0),
    CHECK (position > 0),
    CHECK (octet_length(partition_key) > 0),
    CHECK (jsonb_typeof(trace_carrier) = 'object')
);

CREATE INDEX IF NOT EXISTS event_log_retention_idx
    ON event_log (topic, created_at, partition, position);

CREATE TABLE IF NOT EXISTS event_consumer_offset (
    topic           text        NOT NULL,
    consumer_group  text        NOT NULL,
    partition       integer     NOT NULL,
    position        bigint      NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT '-infinity',
    updated_at      timestamptz NOT NULL DEFAULT clock_timestamp(),

    PRIMARY KEY (topic, consumer_group, partition),

    FOREIGN KEY (topic, partition)
        REFERENCES event_topic_partition (topic, partition),

    CHECK (topic <> ''),
    CHECK (consumer_group <> ''),
    CHECK (partition >= 0),
    CHECK (position >= 0)
);

-- CREATE TABLE IF NOT EXISTS cannot upgrade an existing table. Keep this
-- compatibility step in the adoption baseline for installations created
-- before partition retry eligibility was introduced.
ALTER TABLE event_consumer_offset
    ADD COLUMN IF NOT EXISTS next_attempt_at timestamptz NOT NULL DEFAULT '-infinity';

-- +goose Down

DROP TABLE IF EXISTS event_consumer_offset;
DROP TABLE IF EXISTS event_log;
DROP TABLE IF EXISTS event_topic_partition;
