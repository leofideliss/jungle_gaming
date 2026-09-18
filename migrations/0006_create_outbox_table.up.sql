CREATE TABLE IF NOT EXISTS outbox (
    id UUID PRIMARY KEY,
    event_id UUID NOT NULL,
    aggregate_id UUID NOT NULL,
    aggregate_type VARCHAR(50) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    attempts INT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ,
    locked_by VARCHAR(100),
    locked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uk_event_id UNIQUE (event_id),
    CONSTRAINT ck_outbox_status CHECK (status IN ('PENDING','PUBLISHED','FAILED'))
);

CREATE INDEX idx_outbox_unpublished
    ON outbox (next_attempt_at)
    WHERE status = 'PENDING';
