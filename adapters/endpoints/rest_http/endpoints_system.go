// package: rest_http / transport
// type:    logic
// job:     the operational endpoints — GET /health, /system/whoami and /system/layers
// limits:  translation only; the values come from core.Handle (-> internal/core)
package rest_http

import (
	"net/http"

	"github.com/rankegraph/ranke-db/internal/core"
)

// Health serves GET /health.
func (s *Server) Health(w http.ResponseWriter, r *http.Request) {
	req := &core.Request{Credential: credentialOf(r.Context()), Op: core.OpHealthGet}
	stream, err := s.core.Handle(r.Context(), req)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.respond(w, stream, http.StatusOK)
}

// Whoami serves GET /system/whoami.
func (s *Server) Whoami(w http.ResponseWriter, r *http.Request) {
	req := &core.Request{Credential: credentialOf(r.Context()), Op: core.OpSubjectGet}
	stream, err := s.core.Handle(r.Context(), req)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.respond(w, stream, http.StatusOK)
}

// ListStorageLayers serves GET /system/layers.
func (s *Server) ListStorageLayers(w http.ResponseWriter, r *http.Request) {
	req := &core.Request{Credential: credentialOf(r.Context()), Op: core.OpLayerList}
	stream, err := s.core.Handle(r.Context(), req)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.respond(w, stream, http.StatusOK)
}
