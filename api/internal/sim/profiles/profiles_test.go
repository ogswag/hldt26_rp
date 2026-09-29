package profiles

import "testing"

func TestClassify(t *testing.T) {
	cases := []struct {
		name, family string
		caps         []string
		want         string
		ok           bool
	}{
		{"Ronavi H1500", "AMR", []string{"amr", "payload_pallet"}, AMR, true},
		{"RoboCV stacker", "Автономный штабелёр", nil, Pallet, true},
		{"Погрузчик X", "", []string{"forklift"}, Pallet, true},
		{"RoboCV tug", "Тягач", nil, "", false},
		{"MARK 2 SE", "Робот-уборщик", []string{"cleaning"}, "", false},
	}
	for _, c := range cases {
		got, ok := Classify(c.name, c.family, "", "", c.caps)
		if got != c.want || ok != c.ok {
			t.Fatalf("%s: got %q %v", c.name, got, ok)
		}
	}
}

func TestResolveUsesCatalogAndMarksDefaults(t *testing.T) {
	speed, width := 1.5, 654.0
	p := Resolve(AMR, Specs{SpeedMps: &speed, WidthMm: &width})
	if p.SpeedMps != 1.5 || p.WidthM != 0.654 {
		t.Fatalf("specs not applied: %+v", p)
	}
	if len(p.Assumed) != 4 {
		t.Fatalf("assumed %v", p.Assumed)
	}
	if !p.Can(TaskPalletInbound) || p.Can(TaskPalletPutaway) || !p.Can(TaskPiecePick) {
		t.Fatalf("AMR with 600 kg default payload tasks %v", p.Tasks)
	}
	light := 50.0
	small := Resolve(AMR, Specs{PayloadKg: &light})
	if small.Can(TaskPalletInbound) {
		t.Fatal("50 kg AMR must not move pallets")
	}
	fast := 7.0
	if Resolve(AMR, Specs{SpeedMps: &fast}).SpeedMps != 3 {
		t.Fatal("indoor speed must be capped")
	}
	st := Resolve(Pallet, Specs{})
	if !st.Can(TaskPalletPutaway) || st.Can(TaskPiecePick) {
		t.Fatalf("stacker tasks %v", st.Tasks)
	}
	if st.PickTime(4, true) <= st.PickTime(4, false) || p.DropTime(4, true) != p.DropTime(4, false) {
		t.Fatal("rack handling must add fork time only for pallet robots")
	}
	if st.RequiredWidthM() <= p.RequiredWidthM() {
		t.Fatal("stacker must need a wider aisle than an AMR")
	}
}
