-- +migrate Up

CREATE TABLE "icy_locked_treasuries" (
  "id" SERIAL PRIMARY KEY,
  "address" VARCHAR NOT NULL,
  "created_at" TIMESTAMP NOT NULL DEFAULT now(),
  "updated_at" TIMESTAMP NOT NULL DEFAULT now()
);
