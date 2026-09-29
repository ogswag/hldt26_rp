package simbuild

import (
	"os/exec"
	"strings"
	"testing"
)

// The builder runs in the browser engine too, so it must not reach the database, the queue or HTTP.
func TestBuilderDoesNotReachStorageOrTransport(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("go list: %v", err)
	}
	// NOTE: database/sql/driver is only an interface package that uuid imports; the sql package itself is out.
	forbidden := []string{
		"moscow_hackathon_2026/api/internal/db",
		"moscow_hackathon_2026/api/internal/catalogstore",
		"moscow_hackathon_2026/api/internal/httpserver",
		"moscow_hackathon_2026/api/internal/jobs",
		"moscow_hackathon_2026/api/internal/simjobs",
		"github.com/jackc/pgx",
		"net/http",
	}
	for _, dep := range strings.Fields(string(out)) {
		if dep == "database/sql" {
			t.Errorf("simbuild depends on %s", dep)
		}
		for _, bad := range forbidden {
			if dep == bad || strings.HasPrefix(dep, bad+"/") {
				t.Errorf("simbuild depends on %s", dep)
			}
		}
	}
}
