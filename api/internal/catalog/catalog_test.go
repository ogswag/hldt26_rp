package catalog

import "testing"

func f64(v float64) *float64 { return &v }

func TestDeriveCapabilitiesWarehousePallet(t *testing.T) {
	got := DeriveCapabilities(SpecInput{
		Name:        "Ronavi H1500",
		Subtype:     "AMR",
		ObjectTypes: []string{"warehouse"},
		PayloadKg:   f64(1500),
		WidthMm:     f64(654),
		TempMinC:    f64(5),
		TempMaxC:    f64(25),
	})
	want := []string{CapWarehouseIndoor, CapPayloadPallet, CapPayloadUnit, CapAisleRated, CapTempRated, CapAMR}
	if !containsAll(got, want) {
		t.Fatalf("caps %v want %v", got, want)
	}
}

func TestDeriveCapabilitiesToteNotPallet(t *testing.T) {
	got := DeriveCapabilities(SpecInput{
		Name:        "Tote AMR",
		ObjectTypes: []string{"warehouse"},
		PayloadKg:   f64(50),
		WidthMm:     f64(600),
		MinAisleMm:  f64(900),
	})
	if HasCapability(got, CapPayloadPallet) {
		t.Fatalf("50 kg should not be pallet: %v", got)
	}
	if !HasCapability(got, CapPayloadUnit) {
		t.Fatalf("missing unit cap %v", got)
	}
}

func TestHasCapabilityPalletImpliesUnit(t *testing.T) {
	if !HasCapability([]string{CapPayloadPallet}, CapPayloadUnit) {
		t.Fatal("pallet should cover unit")
	}
}

func TestRequiredCapability(t *testing.T) {
	if RequiredCapability(TaskPalletInbound) != CapPayloadPallet {
		t.Fatal("pallet inbound")
	}
	if RequiredCapability(TaskPiecePick) != CapPayloadUnit {
		t.Fatal("piece pick")
	}
	if RequiredCapability(TaskCleaning) != CapCleaning {
		t.Fatal("cleaning")
	}
	if RequiredCapability("unknown") != "" {
		t.Fatal("unknown")
	}
}

func TestIsCleaner(t *testing.T) {
	if !IsCleaner("MARK 2 SE", "Робот-уборщик", "", "Уборка помещений") {
		t.Fatal("cleaner")
	}
	if IsCleaner("Ronavi H1500", "AMR", "Мобильные роботы", "Внутрискладская логистика") {
		t.Fatal("not cleaner")
	}
}

func containsAll(got, want []string) bool {
	set := make(map[string]struct{}, len(got))
	for _, g := range got {
		set[g] = struct{}{}
	}
	for _, w := range want {
		if _, ok := set[w]; !ok {
			return false
		}
	}
	return true
}
