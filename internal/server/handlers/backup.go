package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/logic-roastery/project-talos/internal/backup"
	"github.com/logic-roastery/project-talos/internal/store"
)

type BackupHandler struct {
	manager     *backup.Manager
	backupStore store.BackupStore
}

func NewBackupHandler(manager *backup.Manager, backupStore store.BackupStore) *BackupHandler {
	return &BackupHandler{manager: manager, backupStore: backupStore}
}

func (h *BackupHandler) Create(w http.ResponseWriter, r *http.Request) {
	b, err := h.manager.CreateFullBackup(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (h *BackupHandler) List(w http.ResponseWriter, r *http.Request) {
	backups, err := h.manager.ListBackups(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, backups)
}

func (h *BackupHandler) Download(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "backupID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid backup id")
		return
	}

	b, err := h.backupStore.GetBackup(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "backup not found")
		return
	}

	backupPath := filepath.Join(h.manager.BackupDir(), b.Filename)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "backup file not found on disk")
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", b.Filename))
	http.ServeFile(w, r, backupPath)
}

func (h *BackupHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "backupID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid backup id")
		return
	}

	if err := h.manager.DeleteBackup(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *BackupHandler) Restore(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "backupID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid backup id")
		return
	}

	if err := h.manager.Restore(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("restore failed: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Backup restored successfully. Restart the server to apply database changes.",
	})
}
