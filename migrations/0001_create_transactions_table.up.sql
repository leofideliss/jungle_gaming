CREATE TABLE IF NOT EXISTS transactions (
    id UUID PRIMARY KEY,

    kind VARCHAR(20) NOT NULL,
    status VARCHAR(20) NOT NULL,

    player_id UUID NOT NULL,
    wallet_id UUID NOT NULL,
    amount BIGINT NOT NULL,
    currency CHAR(3) NOT NULL,

    provider_id VARCHAR(100),
    external_transaction_id VARCHAR(255),
    idempotency_key VARCHAR(512),
    payload_hash VARCHAR(64),
    round_id VARCHAR(255),
    game_id VARCHAR(100),

    reference_external_transaction_id VARCHAR(255),
    reference_transaction_id UUID,

    failure_code VARCHAR(100),

    result_balance BIGINT,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_kind CHECK (
        kind IN ('OPENING', 'BET', 'WIN', 'LOSS', 'REFUND', 'ROLLBACK')
    ),

    CONSTRAINT chk_status CHECK (
        status IN ('PENDING', 'PENDING_REFERENCE', 'PROCESSED', 'REJECTED', 'FAILED')
    ),

    CONSTRAINT chk_amount_non_negative CHECK (amount >= 0),

    CONSTRAINT chk_origin_fields CHECK (
        (
            kind = 'OPENING'
            AND provider_id IS NULL
            AND external_transaction_id IS NULL
            AND idempotency_key IS NULL
            AND payload_hash IS NULL
            AND round_id IS NULL
            AND game_id IS NULL
        )
        OR
        (
            kind <> 'OPENING'
            AND provider_id IS NOT NULL
            AND external_transaction_id IS NOT NULL
            AND idempotency_key IS NOT NULL
            AND payload_hash IS NOT NULL
            AND round_id IS NOT NULL
            AND game_id IS NOT NULL
        )
    ),

    CONSTRAINT chk_reference_required CHECK (
        kind NOT IN ('REFUND', 'ROLLBACK')
        OR reference_external_transaction_id IS NOT NULL
    ),

    CONSTRAINT uq_provider_external UNIQUE (provider_id, external_transaction_id)
);

CREATE UNIQUE INDEX uq_opening_per_wallet
    ON transactions (wallet_id)
    WHERE kind = 'OPENING';

CREATE INDEX idx_tx_provider_external
    ON transactions (provider_id, external_transaction_id);

CREATE INDEX idx_tx_wallet
    ON transactions (wallet_id);
