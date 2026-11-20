-- +goose Up
DROP INDEX trips_active_last_activity_idx;
CREATE INDEX trips_active_last_activity_idx
    ON trips (COALESCE(last_position_at, started_at), id)
    WHERE status = 'active';

-- +goose Down
DROP INDEX trips_active_last_activity_idx;
CREATE INDEX trips_active_last_activity_idx
    ON trips (COALESCE(last_position_at, started_at))
    WHERE status = 'active';
