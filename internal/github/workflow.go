package github

import (
	"fmt"
	"strings"
)

// ProjectType represents the detected project type.
type ProjectType string

const (
	ProjectNode    ProjectType = "node"
	ProjectGo      ProjectType = "go"
	ProjectPython  ProjectType = "python"
	ProjectRust    ProjectType = "rust"
	ProjectRuby    ProjectType = "ruby"
	ProjectUnknown ProjectType = "unknown"
)

// DetectProjectType detects the project type from a list of files in the repo root.
func DetectProjectType(files []string) ProjectType {
	fileSet := make(map[string]bool)
	for _, f := range files {
		fileSet[f] = true
	}

	if fileSet["Dockerfile"] || fileSet["dockerfile"] {
		return ProjectUnknown // Use existing Dockerfile
	}

	if fileSet["package.json"] {
		return ProjectNode
	}
	if fileSet["go.mod"] {
		return ProjectGo
	}
	if fileSet["requirements.txt"] || fileSet["pyproject.toml"] || fileSet["setup.py"] {
		return ProjectPython
	}
	if fileSet["Cargo.toml"] {
		return ProjectRust
	}
	if fileSet["Gemfile"] {
		return ProjectRuby
	}

	return ProjectUnknown
}

// HasDockerfile checks if the file list contains a Dockerfile.
func HasDockerfile(files []string) bool {
	for _, f := range files {
		if strings.EqualFold(f, "Dockerfile") {
			return true
		}
	}
	return false
}

// GenerateDockerfile generates a basic Dockerfile for the given project type.
func GenerateDockerfile(pt ProjectType, port int) string {
	switch pt {
	case ProjectNode:
		return fmt.Sprintf(`FROM node:20-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci
COPY . .

FROM node:20-alpine
WORKDIR /app
COPY --from=builder /app .
EXPOSE %d
CMD ["node", "server.js"]
`, port)

	case ProjectGo:
		return fmt.Sprintf(`FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server .

FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /app
COPY --from=builder /app/server .
EXPOSE %d
CMD ["./server"]
`, port)

	case ProjectPython:
		return fmt.Sprintf(`FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY . .
EXPOSE %d
CMD ["gunicorn", "--bind", "0.0.0.0:%d", "app:app"]
`, port, port)

	case ProjectRust:
		return fmt.Sprintf(`FROM rust:1.77 AS builder
WORKDIR /app
COPY Cargo.toml Cargo.lock ./
COPY src ./src
RUN cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /app/target/release/app .
EXPOSE %d
CMD ["./app"]
`, port)

	case ProjectRuby:
		return fmt.Sprintf(`FROM ruby:3.3-slim
WORKDIR /app
COPY Gemfile Gemfile.lock ./
RUN bundle install
COPY . .
EXPOSE %d
CMD ["bundle", "exec", "puma", "-C", "config/puma.rb"]
`, port)

	default:
		return fmt.Sprintf(`FROM alpine:3.19
WORKDIR /app
COPY . .
EXPOSE %d
CMD ["./start.sh"]
`, port)
	}
}

// WorkflowConfig contains the configuration for generating a GitHub Actions workflow.
type WorkflowConfig struct {
	AppName    string
	ImageRef   string // Full image ref, e.g., ghcr.io/user/repo:tag
	Branch     string
	WebhookURL string // Talos webhook URL
}

// GenerateWorkflow generates a GitHub Actions workflow YAML for deploying to Talos.
func GenerateWorkflow(cfg WorkflowConfig) string {
	return fmt.Sprintf(`name: Deploy to Talos

on:
  push:
    branches: [%s]

env:
  IMAGE_REF: %s

jobs:
  build-and-deploy:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Log in to GitHub Container Registry
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Build and push Docker image
        uses: docker/build-push-action@v5
        with:
          context: .
          push: true
          tags: |
            ${{ env.IMAGE_REF }}
            ghcr.io/${{ github.repository }}:latest

      - name: Notify Talos
        run: |
          curl -X POST "%s/api/webhooks/github" \
            -H "Content-Type: application/json" \
            -H "X-GitHub-Event: workflow_run" \
            -d '{
              "action": "completed",
              "repository": {
                "id": ${{ github.repository_id }},
                "full_name": "${{ github.repository }}",
                "clone_url": "${{ github.server_url }}/${{ github.repository }}.git"
              },
              "workflow_run": {
                "head_branch": "${{ github.ref_name }}",
                "head_sha": "${{ github.sha }}",
                "status": "completed",
                "conclusion": "success"
              }
            }'
`, cfg.Branch, cfg.ImageRef, cfg.WebhookURL)
}
