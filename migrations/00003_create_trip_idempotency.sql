-- +goose Up
CREATE TABLE trip_idempotency (
    key          UUID PRIMARY KEY,
    request_hash BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    trip_id      UUID NOT NULL REFERENCES trips(id),
    expires_at   TIMESTAMPTZ NOT NULL
);

CREATE INDEX trip_idempotency_expires_at_idx
    ON trip_idempotency(expires_at);

-- +goose Down
DROP TABLE trip_idempotency;