package api

import (
	"errors"
	"net/http"
	"strings"

	"ashen/api/internal/store"
)

type createDeviceRequest struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

func (s *Server) handleCreateDevice(w http.ResponseWriter, r *http.Request) {
	var in createDeviceRequest
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if in.Platform == "" {
		in.Platform = "ios"
	}
	d, err := s.store.CreateDevice(r.Context(), userID(r), in.Name, in.Platform)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create device failed")
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

func (s *Server) handleRenameDevice(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	err := s.store.RenameDevice(r.Context(), userID(r), chiURLParam(r, "id"), in.Name)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "device not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, "rename failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.store.ListDevices(r.Context(), userID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list devices failed")
		return
	}
	if devices == nil {
		devices = []store.Device{}
	}
	writeJSON(w, http.StatusOK, devices)
}
