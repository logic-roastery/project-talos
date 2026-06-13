package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/logic-roastery/project-talos/internal/domain"
	"github.com/logic-roastery/project-talos/internal/services"
	"github.com/logic-roastery/project-talos/internal/store"
)

type ServiceHandler struct {
	services    store.ServiceStore
	provisioner *services.Provisioner
}

func NewServiceHandler(services store.ServiceStore, provisioner *services.Provisioner) *ServiceHandler {
	return &ServiceHandler{services: services, provisioner: provisioner}
}

// --- Service CRUD ---

func (h *ServiceHandler) List(w http.ResponseWriter, r *http.Request) {
	svcs, err := h.services.ListServices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, s := range svcs {
		s.Credentials = ""
	}
	writeJSON(w, http.StatusOK, svcs)
}

func (h *ServiceHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "serviceID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid service id")
		return
	}
	svc, err := h.services.GetService(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	svc.Credentials = ""
	writeJSON(w, http.StatusOK, svc)
}

type createServiceRequest struct {
	Name  string                 `json:"name"`
	Type  domain.ServiceType     `json:"type"`
	Creds map[string]interface{} `json:"credentials"`
}

func (h *ServiceHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	def, ok := domain.ServiceDefinitions[req.Type]
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported service type")
		return
	}

	if req.Name == "" {
		req.Name = services.GenerateServiceName(req.Type)
	}

	containerName := fmt.Sprintf("talos-svc-%s", req.Name)

	var creds interface{}
	if req.Creds != nil {
		creds = buildCredsFromMap(req.Type, req.Creds, containerName)
	} else {
		creds = services.DefaultCredentials(req.Type, containerName)
	}

	svc := &domain.Service{
		Name:         req.Name,
		Type:         req.Type,
		ImageRef:     def.DefaultImage,
		Status:       domain.ServiceStatusPending,
		InternalPort: def.Port,
	}

	if err := h.services.CreateService(r.Context(), svc); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			writeError(w, http.StatusConflict, "A service with this name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := h.provisioner.ProvisionService(r.Context(), svc, creds); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("provision failed: %v", err))
		return
	}

	svc.Credentials = ""
	writeJSON(w, http.StatusCreated, svc)
}

func (h *ServiceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "serviceID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid service id")
		return
	}
	if err := h.provisioner.DeleteService(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ServiceHandler) Stop(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "serviceID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid service id")
		return
	}
	svc, err := h.services.GetService(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	if err := h.provisioner.StopService(r.Context(), svc); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	svc.Credentials = ""
	writeJSON(w, http.StatusOK, svc)
}

func (h *ServiceHandler) Start(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "serviceID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid service id")
		return
	}
	svc, err := h.services.GetService(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}
	if err := h.provisioner.StartService(r.Context(), svc); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	svc.Credentials = ""
	writeJSON(w, http.StatusOK, svc)
}

func (h *ServiceHandler) GetCredentials(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "serviceID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid service id")
		return
	}
	svc, err := h.services.GetService(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}

	var creds interface{}
	switch svc.Type {
	case domain.ServicePostgres:
		var pc domain.PostgresCredentials
		if err := h.provisioner.DecryptCredentials(svc, &pc); err != nil {
			writeError(w, http.StatusInternalServerError, "decrypt failed")
			return
		}
		creds = pc
	case domain.ServiceMySQL:
		var mc domain.MySQLCredentials
		if err := h.provisioner.DecryptCredentials(svc, &mc); err != nil {
			writeError(w, http.StatusInternalServerError, "decrypt failed")
			return
		}
		creds = mc
	case domain.ServiceRedis:
		var rc domain.RedisCredentials
		if err := h.provisioner.DecryptCredentials(svc, &rc); err != nil {
			writeError(w, http.StatusInternalServerError, "decrypt failed")
			return
		}
		creds = rc
	case domain.ServiceGarage:
		var gc domain.GarageCredentials
		if err := h.provisioner.DecryptCredentials(svc, &gc); err != nil {
			writeError(w, http.StatusInternalServerError, "decrypt failed")
			return
		}
		creds = gc
	default:
		writeError(w, http.StatusBadRequest, "unknown service type")
		return
	}

	writeJSON(w, http.StatusOK, creds)
}

// --- App-Service Linking ---

type linkServiceRequest struct {
	ServiceID int64  `json:"service_id"`
	Alias     string `json:"alias"`
}

func (h *ServiceHandler) LinkAppService(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(chi.URLParam(r, "appID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app id")
		return
	}
	var req linkServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Alias == "" {
		writeError(w, http.StatusBadRequest, "alias is required")
		return
	}
	if err := h.services.LinkAppService(r.Context(), appID, req.ServiceID, req.Alias); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (h *ServiceHandler) UnlinkAppService(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(chi.URLParam(r, "appID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app id")
		return
	}
	serviceID, err := strconv.ParseInt(chi.URLParam(r, "serviceID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid service id")
		return
	}
	if err := h.services.UnlinkAppService(r.Context(), appID, serviceID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ServiceHandler) ListAppServices(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(chi.URLParam(r, "appID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app id")
		return
	}
	links, err := h.services.ListAppServices(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, links)
}

// --- App Environment Variables ---

type setEnvVarRequest struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	IsSecret bool   `json:"is_secret"`
}

func (h *ServiceHandler) ListEnvVars(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(chi.URLParam(r, "appID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app id")
		return
	}
	vars, err := h.services.GetAppEnvVars(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, v := range vars {
		if v.IsSecret {
			v.Value = "****"
		}
	}
	writeJSON(w, http.StatusOK, vars)
}

func (h *ServiceHandler) SetEnvVar(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(chi.URLParam(r, "appID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app id")
		return
	}
	var req setEnvVarRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	envVar := &domain.AppEnvVar{
		AppID:    appID,
		Key:      req.Key,
		Value:    req.Value,
		IsSecret: req.IsSecret,
	}
	if err := h.services.SetAppEnvVar(r.Context(), envVar); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, envVar)
}

func (h *ServiceHandler) DeleteEnvVar(w http.ResponseWriter, r *http.Request) {
	appID, err := strconv.ParseInt(chi.URLParam(r, "appID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app id")
		return
	}
	key := chi.URLParam(r, "key")
	if err := h.services.DeleteAppEnvVar(r.Context(), appID, key); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ServiceHandler) ListEnvVarHistory(w http.ResponseWriter, r *http.Request) {
	appID, err := parseID(r, "appID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app id")
		return
	}
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	history, err := h.services.GetAppEnvVarHistory(r.Context(), appID, key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get history")
		return
	}
	writeJSON(w, http.StatusOK, history)
}

func (h *ServiceHandler) RevealEnvVar(w http.ResponseWriter, r *http.Request) {
	appID, err := parseID(r, "appID")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid app id")
		return
	}
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	vars, err := h.services.GetAppEnvVars(r.Context(), appID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get env vars")
		return
	}
	for _, v := range vars {
		if v.Key == key {
			writeJSON(w, http.StatusOK, map[string]string{"key": v.Key, "value": v.Value})
			return
		}
	}
	writeError(w, http.StatusNotFound, "env var not found")
}

// --- Helpers ---

func buildCredsFromMap(svcType domain.ServiceType, m map[string]interface{}, containerName string) interface{} {
	switch svcType {
	case domain.ServicePostgres:
		c := domain.PostgresCredentials{
			Host: containerName, Port: 5432, Database: "app", User: "postgres", Password: services.GeneratePassword(24),
		}
		if v, ok := m["database"].(string); ok && v != "" {
			c.Database = v
		}
		if v, ok := m["user"].(string); ok && v != "" {
			c.User = v
		}
		if v, ok := m["password"].(string); ok && v != "" {
			c.Password = v
		}
		return c
	case domain.ServiceMySQL:
		c := domain.MySQLCredentials{
			Host: containerName, Port: 3306, Database: "app", User: "mysql", Password: services.GeneratePassword(24),
		}
		if v, ok := m["database"].(string); ok && v != "" {
			c.Database = v
		}
		if v, ok := m["user"].(string); ok && v != "" {
			c.User = v
		}
		if v, ok := m["password"].(string); ok && v != "" {
			c.Password = v
		}
		return c
	case domain.ServiceRedis:
		c := domain.RedisCredentials{
			Host: containerName, Port: 6379, Password: services.GeneratePassword(24),
		}
		if v, ok := m["password"].(string); ok && v != "" {
			c.Password = v
		}
		return c
	case domain.ServiceGarage:
		c := domain.GarageCredentials{
			Endpoint: fmt.Sprintf("http://%s:3900", containerName), Region: "garage",
			AccessKey: services.GenerateAccessKey(20), SecretKey: services.GeneratePassword(40),
		}
		if v, ok := m["region"].(string); ok && v != "" {
			c.Region = v
		}
		if v, ok := m["access_key"].(string); ok && v != "" {
			c.AccessKey = v
		}
		if v, ok := m["secret_key"].(string); ok && v != "" {
			c.SecretKey = v
		}
		if v, ok := m["bucket"].(string); ok {
			c.Bucket = v
		}
		return c
	}
	return nil
}
