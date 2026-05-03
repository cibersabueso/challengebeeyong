-- Items: catalog with fixed total stock per item.
CREATE TABLE items (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    total       INTEGER NOT NULL CHECK (total >= 0),
    reserved    INTEGER NOT NULL DEFAULT 0 CHECK (reserved >= 0 AND reserved <= total),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Reservations: temporary holds on items with TTL.
CREATE TABLE reservations (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id      UUID NOT NULL REFERENCES items(id),
    user_id      UUID NOT NULL,
    quantity     INTEGER NOT NULL CHECK (quantity > 0),
    status       TEXT NOT NULL CHECK (status IN ('active', 'released', 'expired')),
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at  TIMESTAMPTZ
);

CREATE INDEX idx_reservations_status_expires_at ON reservations (status, expires_at);
CREATE INDEX idx_reservations_user_status        ON reservations (user_id, status);

-- Idempotency keys: client-supplied opaque identifiers for safe retries on POST /reservations.
CREATE TABLE idempotency_keys (
    key             TEXT PRIMARY KEY,
    request_hash    TEXT NOT NULL,
    reservation_id  UUID REFERENCES reservations(id),
    response_status INTEGER NOT NULL,
    response_body   JSONB NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_idempotency_created_at ON idempotency_keys (created_at);
