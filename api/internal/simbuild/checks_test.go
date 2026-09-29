package simbuild

import (
	"encoding/json"
	"testing"

	"moscow_hackathon_2026/api/internal/econ"
	"moscow_hackathon_2026/api/internal/projects"
)

func checksDraft(t *testing.T) projects.Draft {
	t.Helper()
	a := "5760e938-9a43-45a7-b8e8-f4f2e6383930"
	line := func(q int) []projects.FleetItem {
		return []projects.FleetItem{{SolutionID: &a, Quantity: q, TaskCodes: []string{"inbound"}}}
	}
	return projects.NewDraft("warehouse", json.RawMessage(`{"area_total_m2":20000}`), projects.DefaultProcesses("warehouse"), []projects.Variant{
		{ID: "v1", Name: "Первый", Status: "draft", Fleet: line(4)},
		{ID: "v2", Name: "Второй", Status: "draft", Fleet: line(2)},
		{ID: "v3", Name: "Третий", Status: "draft", Fleet: line(1)},
		{ID: "v4", Name: "Пустой", Status: "draft"},
	}, nil)
}

func hashOf(t *testing.T, d projects.Draft, id string) string {
	t.Helper()
	h, err := VariantHash(d, id)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestChecksByVariantHash(t *testing.T) {
	d := checksDraft(t)
	good := &EconCheck{Coverage: 0.97, Text: "Флот выполнил 97% ожидаемых заданий за горизонт, расхождение в пределах 15%."}
	bad := &EconCheck{Coverage: 0.6, Flag: true, Text: "Флот выполнил 60% ожидаемых заданий за горизонт."}
	runs := []RunBrief{
		{RunID: "new-other", VariantID: "v2", VariantHash: "made-for-another-size", EconCheck: bad},
		{RunID: "v1-fresh", VariantID: "v1", VariantHash: hashOf(t, d, "v1"), EconCheck: good},
		{RunID: "v2-fresh", VariantID: "v2", VariantHash: hashOf(t, d, "v2"), EconCheck: good},
		{RunID: "v3-old", VariantID: "v3", VariantHash: "before", EconCheck: bad},
	}
	got, err := Checks(d, runs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("a variant without a fleet has no check, got %+v", got)
	}
	want := []econ.SimCheck{
		{VariantID: "v1", VariantName: "Первый", Model: econ.SimCheckMap, RunID: "v1-fresh", Value: 0.97, Text: good.Text},
		{VariantID: "v2", VariantName: "Второй", Model: econ.SimCheckMap, RunID: "v2-fresh", Value: 0.97, Text: good.Text},
		{VariantID: "v3", VariantName: "Третий", Model: econ.SimCheckMap, RunID: "v3-old", Value: 0.6, Flag: true, Stale: true, Text: bad.Text},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("check %d: %+v, want %+v", i, got[i], want[i])
		}
	}
	if econ.SimChecksFlag(got) {
		t.Error("a flag on a stale run must not raise the verification flag")
	}
}

func TestChecksVariantNeverSimulated(t *testing.T) {
	d := checksDraft(t)
	got, err := Checks(d, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].RunID != "" || got[0].Stale || got[0].Flag {
		t.Fatalf("a variant nobody simulated has an empty check, got %+v", got)
	}
}

func TestChecksEditingOtherVariantKeepsRunCurrent(t *testing.T) {
	d := checksDraft(t)
	runs := []RunBrief{{RunID: "r1", VariantID: "v1", VariantHash: hashOf(t, d, "v1"), EconCheck: &EconCheck{Coverage: 1, Text: "ok"}}}
	d.Variants[1].Fleet[0].Quantity = 30
	got, err := Checks(d, runs)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Stale || got[0].RunID != "r1" {
		t.Fatalf("editing v2 must not make the run of v1 stale: %+v", got[0])
	}
	d.Variants[0].Fleet[0].Quantity = 5
	got, _ = Checks(d, runs)
	if !got[0].Stale {
		t.Fatalf("editing v1 itself must make its run stale: %+v", got[0])
	}
}

func TestChecksRunWithoutDemand(t *testing.T) {
	d := checksDraft(t)
	runs := []RunBrief{{RunID: "r1", VariantID: "v1", VariantHash: hashOf(t, d, "v1")}}
	got, _ := Checks(d, runs)
	if got[0].Text != noDemandText || got[0].Flag {
		t.Fatalf("a run without an economics check says why: %+v", got[0])
	}
}
