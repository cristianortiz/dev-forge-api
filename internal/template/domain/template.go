package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/mod/semver"
)

var (
	ErrTemplateNotFound      = errors.New("template not found")
	ErrTemplateVersionExists = errors.New("template version already exists")
	ErrInvalidTemplate       = errors.New("invalid template")
)

var (
	slugPattern       = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	repoNamePattern   = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9_.-]*[A-Za-z0-9])?$`)
	commitPattern     = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
	sensitiveKeyParts = []string{"password", "secret", "token", "credential", "privatekey", "apikey"}
)

// ProjectTemplate is one immutable, published version of an application blueprint.
type ProjectTemplate struct {
	ID                 uuid.UUID
	Name               string
	Slug               string
	Version            string
	Description        string
	Language           string
	Framework          string
	SourceRepository   string
	SourceCommit       string
	DefaultParams      []byte
	DefaultScopeConfig []byte
	IsActive           bool
	CreatedAt          time.Time
}

// ValidatePublicationMetadata checks source metadata shape and configuration safety.
// GitHub existence and the publication gates are verified outside the API.
func (t *ProjectTemplate) ValidatePublicationMetadata() error {
	if t == nil {
		return fmt.Errorf("%w: template is nil", ErrInvalidTemplate)
	}
	if strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidTemplate)
	}
	if !slugPattern.MatchString(t.Slug) {
		return fmt.Errorf("%w: slug must contain lowercase letters, numbers, and single hyphens", ErrInvalidTemplate)
	}
	if !semver.IsValid(t.Version) {
		return fmt.Errorf("%w: version must be semantic versioning with a v prefix", ErrInvalidTemplate)
	}
	if strings.TrimSpace(t.Language) == "" {
		return fmt.Errorf("%w: language is required", ErrInvalidTemplate)
	}
	if strings.TrimSpace(t.Framework) == "" {
		return fmt.Errorf("%w: framework is required", ErrInvalidTemplate)
	}
	if err := validateSourceRepository(t.SourceRepository); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTemplate, err)
	}
	if !commitPattern.MatchString(t.SourceCommit) {
		return fmt.Errorf("%w: source_commit must be a full 40-character Git SHA", ErrInvalidTemplate)
	}
	if err := validateDefaults("default_params", t.DefaultParams); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTemplate, err)
	}
	if err := validateDefaults("default_scope_config", t.DefaultScopeConfig); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidTemplate, err)
	}
	return nil
}

// SetActive changes only catalog visibility; published metadata is never edited.
func (t *ProjectTemplate) SetActive(active bool) error {
	if t == nil {
		return fmt.Errorf("%w: template is nil", ErrInvalidTemplate)
	}
	if active {
		if err := t.ValidatePublicationMetadata(); err != nil {
			return err
		}
	}
	t.IsActive = active
	return nil
}

func validateSourceRepository(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("source_repository must be an HTTPS GitHub repository URL")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return errors.New("source_repository must identify exactly one GitHub repository")
	}
	if !repoNamePattern.MatchString(parts[0]) || !repoNamePattern.MatchString(parts[1]) {
		return errors.New("source_repository contains an invalid GitHub owner or repository name")
	}
	return nil
}

func validateDefaults(field string, raw []byte) error {
	if len(raw) == 0 {
		return nil
	}

	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return fmt.Errorf("%s must be a JSON object", field)
	}
	if hasSensitiveKey(value) {
		return fmt.Errorf("%s must not contain secret or credential keys", field)
	}
	return nil
}

func hasSensitiveKey(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(key))
			for _, part := range sensitiveKeyParts {
				if strings.Contains(normalized, part) {
					return true
				}
			}
			if hasSensitiveKey(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if hasSensitiveKey(child) {
				return true
			}
		}
	}
	return false
}
