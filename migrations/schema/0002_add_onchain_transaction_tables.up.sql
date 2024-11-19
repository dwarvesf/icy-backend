
CREATE TABLE "onchain_icy_transactions" (
  "id" SERIAL PRIMARY KEY,
  "internal_id" VARCHAR NOT NULL,
  "transaction_hash" VARCHAR NOT NULL,
  "transaction_timestamp" TIMESTAMP NOT NULL,
  "type" VARCHAR NOT NULL,
  "amount" VARCHAR NOT NULL,
  "sender_address" VARCHAR NOT NULL,
  "receiver_address" VARCHAR NOT NULL,
  "created_at" TIMESTAMP NOT NULL DEFAULT now(),
  "updated_at" TIMESTAMP NOT NULL DEFAULT now()
);

CREATE TABLE "onchain_btc_transactions" (
  "id" SERIAL PRIMARY KEY,
  "internal_id" VARCHAR NOT NULL,
  "transaction_hash" VARCHAR NOT NULL,
  "transaction_timestamp" TIMESTAMP NOT NULL,
  "type" VARCHAR NOT NULL,
  "amount" VARCHAR NOT NULL,
  "sender_address" VARCHAR NOT NULL,
  "receiver_address" VARCHAR NOT NULL,
  "created_at" TIMESTAMP NOT NULL DEFAULT now(),
  "updated_at" TIMESTAMP NOT NULL DEFAULT now()
);
