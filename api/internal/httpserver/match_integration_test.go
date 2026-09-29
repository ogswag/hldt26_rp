package httpserver

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"moscow_hackathon_2026/api/internal/matching"
	"moscow_hackathon_2026/api/internal/ops"
)

func TestProjectMatchPreviewSavesNothing(t *testing.T) {
	env := newEnv(t)
	owner := register(t, env.h, "preview@example.com")
	p, _ := warehouseProject(t, owner, "Предпросмотр подбора")
	postTxs(t, owner, p.ID, newTx(opSet(ops.CollProject, ops.CollProject, "match_selected_ids", []string{stackerID})))

	ctx := context.Background()
	state := func() (doc, hash string, versions int) {
		t.Helper()
		if err := env.db.QueryRow(ctx, `SELECT d.document::text, d.input_hash,
			(SELECT count(*) FROM project_versions v WHERE v.project_id = d.project_id)
			FROM project_drafts d WHERE d.project_id = $1`, p.ID).Scan(&doc, &hash, &versions); err != nil {
			t.Fatal(err)
		}
		return doc, hash, versions
	}
	doc0, hash0, versions0 := state()

	var brief, full matching.Output
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/match?view=brief", nil, http.StatusOK, &brief)
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/match", nil, http.StatusOK, &full)
	owner.json(http.MethodGet, "/api/projects/"+p.ID+"/match?view=short", nil, http.StatusBadRequest, nil)

	if len(brief.Items) == 0 || len(brief.Items) != len(full.Items) {
		t.Fatalf("items brief %d full %d", len(brief.Items), len(full.Items))
	}
	if !slices.Equal(brief.SelectedIDs, []string{stackerID}) {
		t.Fatalf("stored selection not used: %v", brief.SelectedIDs)
	}
	if want := []string{"pallet_inbound", "pallet_putaway", "piece_pick", "pallet_outbound"}; !slices.Equal(brief.TaskCodes, want) {
		t.Fatalf("task codes %v, want %v", brief.TaskCodes, want)
	}
	traced := false
	for i, it := range brief.Items {
		ref := full.Items[i]
		if it.SolutionID != ref.SolutionID || it.Status != ref.Status || it.Score != ref.Score {
			t.Fatalf("item %d differs between views: %+v vs %+v", i, it, ref)
		}
		if len(it.Reasons) != 0 || len(it.Explanation) != 0 || len(it.Hard) != 0 {
			t.Fatalf("brief item %s keeps its trace", it.SolutionID)
		}
		if it.Forced != (it.SolutionID == stackerID) {
			t.Fatalf("forced flag on %s: %v", it.SolutionID, it.Forced)
		}
		traced = traced || len(ref.Explanation) > 0
	}
	if !traced {
		t.Fatal("full view has no explanation chain")
	}

	doc1, hash1, versions1 := state()
	if doc1 != doc0 || hash1 != hash0 || versions1 != versions0 {
		t.Fatalf("GET match changed the project: hash %s -> %s, versions %d -> %d", hash0, hash1, versions0, versions1)
	}
}
