
CREATE TABLE "onchain_icy_transactions" (
  "id" SERIAL PRIMARY KEY,
  "internal_id" VARCHAR NOT NULL,
  "transaction_hash" VARCHAR NOT NULL,
  "block_time" INTEGER,
  "type" VARCHAR NOT NULL,
  "amount" VARCHAR NOT NULL,
  "other_address" VARCHAR NOT NULL,
  "fee" VARCHAR,
  "created_at" TIMESTAMP NOT NULL DEFAULT now(),
  "updated_at" TIMESTAMP NOT NULL DEFAULT now()
);

CREATE TABLE "onchain_btc_transactions" (
  "id" SERIAL PRIMARY KEY,
  "internal_id" VARCHAR NOT NULL,
  "transaction_hash" VARCHAR NOT NULL,
  "block_time" INTEGER,
  "type" VARCHAR NOT NULL,
  "amount" VARCHAR NOT NULL,
  "other_address" VARCHAR NOT NULL,
  "fee" VARCHAR,
  "created_at" TIMESTAMP NOT NULL DEFAULT now(),
  "updated_at" TIMESTAMP NOT NULL DEFAULT now()
);
