-- Development migration: replace the prototype catalog and discard its rows.
DROP TABLE project_templates;

CREATE TABLE project_templates (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                 VARCHAR(255) NOT NULL,
    slug                 VARCHAR(100) NOT NULL,
    version              VARCHAR(50) NOT NULL,
    description          TEXT NOT NULL DEFAULT '',
    language             VARCHAR(50) NOT NULL,
    framework            VARCHAR(100) NOT NULL,
    source_repository    VARCHAR(500) NOT NULL,
    source_commit        VARCHAR(40) NOT NULL,
    default_params       JSONB NOT NULL DEFAULT '{}',
    default_scope_config JSONB NOT NULL DEFAULT '{}',
    is_active            BOOLEAN NOT NULL DEFAULT TRUE,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT project_templates_slug_version_key UNIQUE (slug, version),
    CONSTRAINT project_templates_slug_format_check
        CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    CONSTRAINT project_templates_version_format_check
        CHECK (version ~ '^v(0|[1-9][0-9]*)[.](0|[1-9][0-9]*)[.](0|[1-9][0-9]*)([-+][0-9A-Za-z.-]+)?$'),
    CONSTRAINT project_templates_source_repository_check
        CHECK (
            source_repository ~* '^https://github[.]com/[A-Za-z0-9]([A-Za-z0-9_.-]*[A-Za-z0-9])?/[A-Za-z0-9]([A-Za-z0-9_.-]*[A-Za-z0-9])?$'
        ),
    CONSTRAINT project_templates_source_commit_check
        CHECK (source_commit ~ '^[0-9a-fA-F]{40}$'),
    CONSTRAINT project_templates_default_params_object_check
        CHECK (jsonb_typeof(default_params) = 'object'),
    CONSTRAINT project_templates_default_scope_config_object_check
        CHECK (jsonb_typeof(default_scope_config) = 'object')
);

CREATE INDEX idx_project_templates_language ON project_templates (language);
CREATE INDEX idx_project_templates_is_active ON project_templates (is_active);
