package server

import (
	"strings"

	"github.com/tidwall/redcon"

	"github.com/elcuervo/emb/internal/contract"
)

// doc resolves a wire name to its contract doc, panicking on a missing one:
// a route without a doc is a programming error, not a runtime condition.
func doc(name string) contract.Doc {
	d, ok := contract.ByName(name)
	if !ok {
		panic("server: no contract doc for " + name)
	}
	return d
}

// route registers h and records d so the contract tests can prove every served
// command is documented.
func (s *Server) route(mux *redcon.ServeMux, d contract.Doc, h redcon.HandlerFunc) {
	mux.HandleFunc(strings.ToLower(d.Name), h)
	s.routes = append(s.routes, d)
}
