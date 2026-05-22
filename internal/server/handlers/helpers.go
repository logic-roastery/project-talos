package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/logic-roastery/project-talos/internal/domain"
)

type ContextKey string

const UserCtxKey ContextKey = "user"

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func UserFromContext(ctx context.Context) *domain.User {
	if user, ok := ctx.Value(UserCtxKey).(*domain.User); ok {
		return user
	}
	return nil
}
