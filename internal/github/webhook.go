package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// WebhookEvent represents the type of GitHub webhook event.
type WebhookEvent string

const (
	EventWorkflowRun  WebhookEvent = "workflow_run"
	EventInstallation WebhookEvent = "installation"
	EventPush         WebhookEvent = "push"
)

// WebhookResult contains the parsed webhook data.
type WebhookResult struct {
	Event   WebhookEvent
	Payload []byte
}

// WorkflowRunPayload represents a workflow_run webhook event.
type WorkflowRunPayload struct {
	Action       string     `json:"action"`
	Repository   Repository `json:"repository"`
	Workflow     Workflow   `json:"workflow_run"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
}

// InstallationPayload represents an installation webhook event.
type InstallationPayload struct {
	Action       string              `json:"action"`
	Installation Installation        `json:"installation"`
	Repositories []RepositorySummary `json:"repositories"`
}

// PushPayload represents a push webhook event.
type PushPayload struct {
	Ref          string     `json:"ref"`
	After        string     `json:"after"`
	Repository   Repository `json:"repository"`
	Installation struct {
		ID int64 `json:"id"`
	} `json:"installation"`
}

// Installation contains the installation details.
type Installation struct {
	ID int64 `json:"id"`
}

// RepositorySummary is a minimal repo reference in installation events.
type RepositorySummary struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	NodeID   string `json:"node_id"`
}

// Repository contains repository details from webhook payloads.
type Repository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	CloneURL string `json:"clone_url"`
}

// Workflow contains workflow_run details.
type Workflow struct {
	HeadBranch string `json:"head_branch"`
	HeadSHA    string `json:"head_sha"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

// WebhookHandler handles GitHub webhook verification and parsing.
type WebhookHandler struct {
	secret string
}

// NewWebhookHandler creates a new webhook handler with the given secret.
func NewWebhookHandler(secret string) *WebhookHandler {
	return &WebhookHandler{secret: secret}
}

// VerifyAndParse verifies the webhook signature and returns the raw body and event type.
func (h *WebhookHandler) VerifyAndParse(r *http.Request) (*WebhookResult, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	defer r.Body.Close()

	if h.secret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !h.verifySignature(body, sig) {
			return nil, fmt.Errorf("invalid signature")
		}
	}

	eventType := WebhookEvent(r.Header.Get("X-GitHub-Event"))
	if eventType == "" {
		return nil, fmt.Errorf("missing X-GitHub-Event header")
	}

	return &WebhookResult{
		Event:   eventType,
		Payload: body,
	}, nil
}

// ParseWorkflowRun parses a workflow_run event payload.
func ParseWorkflowRun(payload []byte) (*WorkflowRunPayload, error) {
	var p WorkflowRunPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("parse workflow_run: %w", err)
	}
	return &p, nil
}

// ParseInstallation parses an installation event payload.
func ParseInstallation(payload []byte) (*InstallationPayload, error) {
	var p InstallationPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("parse installation: %w", err)
	}
	return &p, nil
}

// ParsePush parses a push event payload.
func ParsePush(payload []byte) (*PushPayload, error) {
	var p PushPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("parse push: %w", err)
	}
	return &p, nil
}

func (h *WebhookHandler) verifySignature(body []byte, signature string) bool {
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(body)
	expected := mac.Sum(nil)

	return hmac.Equal(sig, expected)
}
