package openapi

import _ "embed"

// Spec is the canonical OpenAPI contract served by the API.
//
//go:embed openapi.yaml
var Spec []byte
