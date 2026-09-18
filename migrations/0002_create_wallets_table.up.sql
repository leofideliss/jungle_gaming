CREATE TABLE IF NOT EXISTS wallets (
id UUID PRIMARY KEY,
player_id UUID NOT NULL,
currency CHAR(3) NOT NULL,
balance BIGINT NOT NULL,
version BIGINT DEFAULT 1 NOT NULL,
created_at TIMESTAMPTZ DEFAULT NOW(),
updated_at TIMESTAMPTZ DEFAULT NOW(),

CONSTRAINT uk_player_currency UNIQUE(player_id,currency),
CONSTRAINT ck_balance CHECK (balance >= 0)

);

