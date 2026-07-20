-- A payout that fails before the broadcast POST is released back to "pending"
-- and retried on the next settlement tick. Nothing bounded that, so a row whose
-- failure is permanent (treasury empty, endpoints down, an amount the network
-- will never accept) re-failed forever, burning a UTXO fetch and a fee estimate
-- every tick with the user's ICY already burned.
--
-- DEFAULT 0 and NOT NULL so existing rows start at zero and the retry budget
-- applies to them from the next tick onwards.
ALTER TABLE onchain_btc_processed_transactions
ADD COLUMN IF NOT EXISTS attempts INTEGER NOT NULL DEFAULT 0;
