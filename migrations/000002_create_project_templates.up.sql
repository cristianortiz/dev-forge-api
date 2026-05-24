-- Migration: create project_templates table
-- Phase 1, task 1.10

CREATE TABLE project_templates (
    id                   UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    name                 VARCHAR(255) NOT NULL,
    slug                 VARCHAR(100) NOT NULL UNIQUE,
    description          TEXT         NOT NULL DEFAULT '',
    language             VARCHAR(50)  NOT NULL,
    framework            VARCHAR(100) NOT NULL DEFAULT '',
    dockerfile_template  TEXT         NOT NULL,
    default_params       JSONB        NOT NULL DEFAULT '{}',
    default_scope_config JSONB        NOT NULL DEFAULT '{}',
    repo_template_url    VARCHAR(500) NOT NULL DEFAULT '',
    is_active            BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_project_templates_language  ON project_templates (language);
CREATE INDEX idx_project_templates_is_active ON project_templates (is_active);
