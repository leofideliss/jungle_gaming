CREATE TABLE IF NOT EXISTS wallet_ledger_entries (
id UUID PRIMARY KEY,
wallet_id UUID NOT NULL,
transaction_id UUID NOT NULL,
movement_type VARCHAR(20) NOT NULL,
amount BIGINT NOT NULL,
before_amount BIGINT NOT NULL,
after_amount BIGINT NOT NULL,

created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
CONSTRAINT ck_after_value CHECK (
(movement_type = 'CREDIT' AND after_amount = before_amount + amount)
OR
(movement_type = 'DEBIT'  AND after_amount = before_amount - amount)
),

CONSTRAINT uk_transaction_wallet_id UNIQUE(transaction_id,wallet_id),

CONSTRAINT ck_movement_type CHECk (movement_type = 'CREDIT' OR movement_type = 'DEBIT')  
);
