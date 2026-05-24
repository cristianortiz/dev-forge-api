package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Domain errors.
var (
	ErrTemplateNotFound  = errors.New("template not found")
	ErrTemplateInactive  = errors.New("template is inactive")
	ErrSlugAlreadyExists = errors.New("template slug already exists")
)

// ProjectTemplate defines the scaffold for a type of application.
// It holds the Dockerfile template and default configuration applied
// when creating an Application of this type.
type ProjectTemplate struct {
	ID                 uuid.UUID
	Name               string // "Go REST API", "React SPA"
	Slug               string // "go-rest-api", "react-spa" — unique, URL-safe
	Description        string
	Language           string // "go", "javascript", "python"
	Framework          string // "fiber", "react", "fastapi"
	DockerfileTemplate string // Dockerfile contents injected at Docker build time
	DefaultParams      []byte // JSONB: default env vars map
	DefaultScopeConfig []byte // JSONB: default RAM, CPU, replicas, health check path
	RepoTemplateURL    string // optional GitHub template repo URL
	IsActive           bool
	CreatedAt          time.Time
}
