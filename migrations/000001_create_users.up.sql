-- Migration: create users table
-- Phase 1, task 1.10

CREATE TABLE users (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    zitadel_id   VARCHAR(255) UNIQUE NOT NULL,  -- sub claim from Zitadel JWT
    email        VARCHAR(255) UNIQUE NOT NULL,
    name         VARCHAR(255) NOT NULL,
    role         VARCHAR(20)  NOT NULL DEFAULT 'developer', -- admin | developer | viewer
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
