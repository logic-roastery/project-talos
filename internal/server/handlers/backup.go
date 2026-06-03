package handlers

import (
	"net/http"

	"github.com/logic-roastery/project-talos/internal/backup"
)

type BackupHandler struct {
	manager *backup.Manager
}

func NewBackupHandler(manager *backup.Manager) *BackupHandler {
	return &BackupHandler{manager: manager}
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
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *BackupHandler) Restore(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "backupID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid backup id")
		return
	}

	if err := h.manager.Restore(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "restored", "message": "restore complete — restart the process to apply"})
}
