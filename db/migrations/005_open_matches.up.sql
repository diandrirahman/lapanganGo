-- Migration: Create open_matches and open_match_participants tables

CREATE TABLE IF NOT EXISTS open_matches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id UUID NOT NULL UNIQUE REFERENCES bookings(id) ON DELETE CASCADE,
    host_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    title VARCHAR(100) NOT NULL,
    description TEXT,
    level VARCHAR(50) NOT NULL,
    max_players INTEGER NOT NULL,
    price_per_player NUMERIC(12, 2) NOT NULL DEFAULT 0,
    status VARCHAR(50) NOT NULL DEFAULT 'OPEN',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT valid_level CHECK (level IN ('Beginner / Fun', 'Intermediate', 'Advanced', 'All Levels')),
    CONSTRAINT valid_status CHECK (status IN ('OPEN', 'FULL', 'CANCELLED', 'COMPLETED')),
    CONSTRAINT positive_max_players CHECK (max_players > 0),
    CONSTRAINT non_negative_price CHECK (price_per_player >= 0)
);

CREATE TABLE IF NOT EXISTS open_match_participants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    open_match_id UUID NOT NULL REFERENCES open_matches(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status VARCHAR(50) NOT NULL DEFAULT 'JOINED',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    cancelled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(open_match_id, user_id),
    CONSTRAINT valid_participant_status CHECK (status IN ('JOINED', 'CANCELLED'))
);

CREATE INDEX IF NOT EXISTS idx_open_matches_booking_id ON open_matches(booking_id);
CREATE INDEX IF NOT EXISTS idx_open_matches_host_user_id ON open_matches(host_user_id);
CREATE INDEX IF NOT EXISTS idx_open_matches_status ON open_matches(status);

CREATE INDEX IF NOT EXISTS idx_open_match_participants_match_id ON open_match_participants(open_match_id);
CREATE INDEX IF NOT EXISTS idx_open_match_participants_user_id ON open_match_participants(user_id);
