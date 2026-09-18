CREATE TABLE IF NOT EXISTS inbox (
id UUID PRIMARY KEY,
msg_id VARCHAR(255) NOT NULL ,
consumer_id VARCHAR(255) NOT NULL,
json_hash_hex CHAR(64) NOT NULL,
received_at TIMESTAMPTZ DEFAULT NOW(),
finished_at TIMESTAMPTZ DEFAULT NULL,

CONSTRAINT uk_msg_id_consumer_id UNIQUE(msg_id,consumer_id)
);
