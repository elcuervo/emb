package server

import (
	"strings"
	"testing"

	"github.com/elcuervo/emb/internal/contract"
)

// TestContractRoutesMatchDocs proves the server serves exactly the docs and
// nothing twice.
func TestContractRoutesMatchDocs(t *testing.T) {
	_, srv := serveTestWithServer(t)

	served := make(map[string]int, len(srv.routes))
	for _, d := range srv.routes {
		served[d.Name]++
	}
	documented := make(map[string]bool, len(contract.Docs))
	for _, d := range contract.Docs {
		documented[d.Name] = true
	}

	for _, d := range contract.Docs {
		if served[d.Name] == 0 {
			t.Errorf("documented command %s has no route", d.Name)
		}
	}
	for name, n := range served {
		if !documented[name] {
			t.Errorf("route %s has no contract doc", name)
		}
		if n > 1 {
			t.Errorf("route %s registered %d times", name, n)
		}
	}
}

// TestHelpListsEveryCommand proves EMB.HELP advertises every served command.
// Command lines start with the wire name as their first token; a substring test
// would let EMB.INFO pass for a missing INFO entry.
func TestHelpListsEveryCommand(t *testing.T) {
	addr, _ := serveTestWithServer(t)
	help := bulkOf(t, redisCmd(t, addr, "EMB.HELP"))
	named := make(map[string]bool)
	for _, line := range strings.Split(help, "\n") {
		if fields := strings.Fields(line); len(fields) > 0 {
			named[fields[0]] = true
		}
	}
	for _, d := range contract.Docs {
		if !named[d.Name] {
			t.Errorf("EMB.HELP omits %s", d.Name)
		}
	}
}
