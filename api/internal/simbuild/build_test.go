package simbuild

import (
	"encoding/json"
	"math"
	"testing"

	"moscow_hackathon_2026/api/internal/projects"
)

func TestReadParams(t *testing.T) {
	p := readParams(json.RawMessage(`{"peak_load_factor":1.8,"shifts_per_day":2,"shift_hours":11,
		"shift_window":{"start":"08:00","end":"22:00"},"aisle_main_m":3.2,"aisle_working_m":2.6,"ceiling_height_m":10}`))
	if p.peak != 1.8 || p.windowH != 14 || p.shiftsH != 22 || p.mainAisle != 3.2 || p.liftM != 4 || p.liftAssumed {
		t.Fatalf("params %+v", p)
	}
	active, note := p.activeHours()
	if active != 14 || note == "" {
		t.Fatalf("active %v note %q", active, note)
	}
	night := readParams(json.RawMessage(`{"shift_window":{"start":"22:00","end":"06:30"}}`))
	if math.Abs(night.windowH-8.5) > 1e-9 || night.peak != 1.5 || !night.liftAssumed {
		t.Fatalf("overnight %+v", night)
	}
	if h, _ := readParams(json.RawMessage(`{"shifts_per_day":3,"shift_hours":10}`)).activeHours(); h != 24 {
		t.Fatalf("shifts capped at 24, got %v", h)
	}
	if _, ok := clockHours("25:00"); ok {
		t.Fatal("bad clock accepted")
	}
	if h, ok := clockHours("08:00:00"); !ok || h != 8 {
		t.Fatalf("seconds clock %v %v", h, ok)
	}
}

func TestPickVariant(t *testing.T) {
	sid := "5760e938-9a43-45a7-b8e8-f4f2e6383930"
	d := projects.Draft{Variants: []projects.Variant{
		{ID: "a", Name: "Пустой", Fleet: []projects.FleetItem{}},
		{ID: "b", Name: "AMR", Fleet: []projects.FleetItem{{SolutionID: &sid, Quantity: 2}}},
	}}
	v, err := pickVariant(d, "")
	if err != nil || v.ID != "b" {
		t.Fatalf("default variant %v %v", v.ID, err)
	}
	if _, err := pickVariant(d, "a"); err == nil {
		t.Fatal("empty variant accepted")
	}
	if _, err := pickVariant(d, "zzz"); err == nil {
		t.Fatal("unknown variant accepted")
	}
	if _, ok := UserMessage(func() error { _, err := pickVariant(d, "a"); return err }()); !ok {
		t.Fatal("variant error must be shown to the user")
	}
}

func TestJobVariantRunsTheSuggestionWithoutFleet(t *testing.T) {
	d := projects.Draft{Variants: []projects.Variant{{ID: "a", Name: "Вариант 1", Fleet: []projects.FleetItem{}}}}
	if HasFleet(d) {
		t.Fatal("empty variants have a fleet")
	}
	v, err := jobVariant(d, JobConfig{Suggestion: &Suggestion{SolutionID: "5760e938-9a43-45a7-b8e8-f4f2e6383930", Quantity: 13}})
	if err != nil || v.ID != "" || v.Name != projects.SuggestionName || len(v.Fleet) != 1 || v.Fleet[0].Quantity != 13 {
		t.Fatalf("suggestion variant %+v %v", v, err)
	}
	if _, err := jobVariant(d, JobConfig{}); err == nil {
		t.Fatal("a job without fleet or suggestion was accepted")
	}
}

func TestTemplateUsesProjectWidthsAndProcesses(t *testing.T) {
	d := projects.NewDraft("warehouse", json.RawMessage(`{"aisle_main_m":4.1}`), projects.DefaultProcesses("warehouse")[:2], nil, nil)
	doc := Template(d)
	if len(doc.Layers.Flows) != 2 {
		t.Fatalf("flows %d", len(doc.Layers.Flows))
	}
	found := false
	for _, e := range doc.Layers.Edges {
		if e.ID == "e-ma1" && e.WidthM != nil && *e.WidthM == 4.1 {
			found = true
		}
	}
	if !found || DefaultWidthM(d) != 4.1 {
		t.Fatal("main aisle width must come from params")
	}
	doc2, source, err := MapFor(d)
	if err != nil || source != MapSourceTemplate || len(doc2.Layers.Points) == 0 {
		t.Fatalf("map for empty draft: %v %v", source, err)
	}
	d.Map = json.RawMessage(`{"schema_version":"map-v1","bogus":1}`)
	if _, _, err := MapFor(d); err == nil {
		t.Fatal("broken stored map accepted")
	}
}
