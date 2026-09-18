ALTER TABLE transactions ADD CONSTRAINT fk_transactions_wallet_id FOREIGN KEY (wallet_id) REFERENCES wallets(id);
