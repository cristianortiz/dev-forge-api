package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cristianortiz/dev-forge/internal/shared/config"
)

// seed inserts the five built-in project templates.
// Idempotent: uses ON CONFLICT (slug) DO NOTHING, so it is safe to run multiple times.
func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.Database.URL())
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect db: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	const query = `
INSERT INTO project_templates
  (name, slug, description, language, framework, dockerfile_template, default_params, default_scope_config, repo_template_url)
VALUES
  ($1, $2, $3, $4, $5, $6, $7::jsonb, $8::jsonb, $9)
ON CONFLICT (slug) DO NOTHING`

	templates := []struct {
		name, slug, description, language, framework string
		dockerfile                                   string
		defaultParams, defaultScopeConfig            string
		repoURL                                      string
	}{
		{
			name:        "Go REST API",
			slug:        "go-rest-api",
			description: "Production-ready Go microservice — hexagonal architecture, Fiber, Zitadel JWT auth, OTEL traces/metrics, Swagger, pgx, GitHub Actions CI.",
			language:    "go",
			framework:   "fiber",
			dockerfile: `FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server ./cmd/server
FROM alpine:3.19
RUN addgroup -S app && adduser -S app -G app
COPY --from=builder /app/server /server
USER app
EXPOSE 8080
CMD ["/server"]`,
			defaultParams:      `{"PORT":"8080","ENV":"production","LOG_LEVEL":"info"}`,
			defaultScopeConfig: `{"replicas":2,"cpu":"250m","memory":"256Mi","health_check_path":"/health"}`,
			repoURL:            "",
		},
		{
			name:        "React SPA",
			slug:        "react-spa",
			description: "Production-ready React SPA — React 18 + Vite + TypeScript, Zitadel OIDC PKCE, TanStack Query, shadcn/ui, OTEL Web SDK, GitHub Actions CI.",
			language:    "javascript",
			framework:   "react",
			dockerfile: `FROM node:22-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build
FROM nginx:1.27-alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]`,
			defaultParams:      `{"NODE_ENV":"production","VITE_API_URL":"https://api.example.com"}`,
			defaultScopeConfig: `{"replicas":2,"cpu":"100m","memory":"128Mi","health_check_path":"/"}`,
			repoURL:            "",
		},
		{
			name:        "Node.js Express API",
			slug:        "nodejs-express",
			description: "REST API with Node.js, Express, and TypeScript",
			language:    "javascript",
			framework:   "express",
			dockerfile: `FROM node:22-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .
RUN npm run build
FROM node:22-alpine
WORKDIR /app
COPY package*.json ./
RUN npm ci --only=production
COPY --from=builder /app/dist ./dist
EXPOSE 3000
CMD ["node", "dist/index.js"]`,
			defaultParams:      `{"PORT":"3000","NODE_ENV":"production","LOG_LEVEL":"info"}`,
			defaultScopeConfig: `{"replicas":2,"cpu":"200m","memory":"256Mi","health_check_path":"/health"}`,
			repoURL:            "",
		},
		{
			name:        "Python FastAPI",
			slug:        "python-fastapi",
			description: "Async REST API with Python 3.13, FastAPI, and Pydantic v2",
			language:    "python",
			framework:   "fastapi",
			dockerfile: `FROM python:3.13-slim AS builder
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir --upgrade pip \
    && pip install --no-cache-dir -r requirements.txt
COPY . .
FROM python:3.13-slim
WORKDIR /app
COPY --from=builder /app /app
RUN useradd -m appuser
USER appuser
EXPOSE 8000
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]`,
			defaultParams:      `{"PORT":"8000","ENV":"production","LOG_LEVEL":"info"}`,
			defaultScopeConfig: `{"replicas":2,"cpu":"250m","memory":"512Mi","health_check_path":"/health"}`,
			repoURL:            "",
		},
		{
			name:        "Go Modular Monolith",
			slug:        "go-modular-monolith",
			description: "Production-ready Go modular monolith — multiple domain modules, hexagonal architecture, Zitadel auth, OTEL, Swagger. Same structure as dev-forge.",
			language:    "go",
			framework:   "fiber",
			dockerfile: `FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server ./cmd/server
FROM alpine:3.19
RUN addgroup -S app && adduser -S app -G app
COPY --from=builder /app/server /server
USER app
EXPOSE 8080
CMD ["/server"]`,
			defaultParams:      `{"PORT":"8080","ENV":"production","LOG_LEVEL":"info"}`,
			defaultScopeConfig: `{"replicas":2,"cpu":"500m","memory":"512Mi","health_check_path":"/health"}`,
			repoURL:            "",
		},
	}

	inserted := 0
	for _, t := range templates {
		tag, err := pool.Exec(ctx, query,
			t.name, t.slug, t.description, t.language, t.framework,
			t.dockerfile, t.defaultParams, t.defaultScopeConfig, t.repoURL,
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "insert %q: %v\n", t.slug, err)
			os.Exit(1)
		}
		if tag.RowsAffected() > 0 {
			inserted++
			fmt.Printf("  ✓ inserted  %s (%s)\n", t.slug, t.language)
		} else {
			fmt.Printf("  - skipped   %s (already exists)\n", t.slug)
		}
	}
	fmt.Printf("\nDone. %d/%d templates inserted.\n", inserted, len(templates))
}
