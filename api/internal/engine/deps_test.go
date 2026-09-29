package engine_test

import (
	"os/exec"
	"strings"
	"testing"
)

// trees the engine must not reach, so the same code can run in a browser build.
var forbiddenTrees = []string{
	"moscow_hackathon_2026/api/internal/db",
	"moscow_hackathon_2026/api/internal/catalogstore",
	"moscow_hackathon_2026/api/internal/httpserver",
	"moscow_hackathon_2026/api/internal/jobs",
	"moscow_hackathon_2026/api/internal/simjobs",
	"github.com/jackc/pgx",
	"net/http",
}

// packages the engine must not reach. database/sql/driver is allowed: google/uuid declares its Scanner there
// and no driver comes with it.
var forbiddenPackages = []string{"database/sql"}

func TestEngineHasNoServerDependencies(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "moscow_hackathon_2026/api/internal/engine").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, dep := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		for _, bad := range forbiddenPackages {
			if dep == bad {
				t.Errorf("engine depends on %s", dep)
			}
		}
		for _, bad := range forbiddenTrees {
			if dep == bad || strings.HasPrefix(dep, bad+"/") {
				t.Errorf("engine depends on %s", dep)
			}
		}
	}
}
