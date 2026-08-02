package domain

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

type Application struct {
	ID                 uuid.UUID
	Name               string
	Slug               string // kebab-case, único
	Description        string
	OwnerID            uuid.UUID  // FK users.id
	TemplateID         *uuid.UUID // nullable
	RepoURL            string
	Language           string
	Framework          string
	DefaultParams      json.RawMessage
	DefaultScopeConfig json.RawMessage
	IsActive           bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// domain errores
var (
	ErrNotFound    = errors.New("application not found")
	ErrSlugTaken   = errors.New("slug already in use")
	ErrInvalidName = errors.New("name is required")
)
