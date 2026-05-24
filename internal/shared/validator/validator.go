// Package validator provides struct-level validation using `validate` struct tags.
// Fiber-specific middleware (ValidateBody, GetBody) lives in shared/middleware.
package validator

import (
	"errors"
	"fmt"
	"strings"

	v10 "github.com/go-playground/validator/v10"
)

var instance = v10.New()

// Struct validates s using its `validate` struct tags.
// Returns a human-readable error joining all field failures, e.g. "name: required; language: required".
func Struct(s any) error {
	if err := instance.Struct(s); err != nil {
		var verrs v10.ValidationErrors
		if errors.As(err, &verrs) {
			msgs := make([]string, 0, len(verrs))
			for _, e := range verrs {
				msgs = append(msgs, fmt.Sprintf("%s: %s", strings.ToLower(e.Field()), e.Tag()))
			}
			return fmt.Errorf("%s", strings.Join(msgs, "; "))
		}
		return err
	}
	return nil
}
