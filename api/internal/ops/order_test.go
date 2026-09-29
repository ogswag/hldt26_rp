package ops

import "testing"

func TestKeyBetween(t *testing.T) {
	for _, c := range []struct {
		a, b, want string
		fails      bool
	}{
		{"", "", "a0", false},
		{"", "a0", "Zz", false},
		{"", "Zz", "Zy", false},
		{"a0", "", "a1", false},
		{"a1", "", "a2", false},
		{"a0", "a1", "a0V", false},
		{"a1", "a2", "a1V", false},
		{"a0V", "a1", "a0l", false},
		{"Zz", "a0", "ZzV", false},
		{"Zz", "a1", "a0", false},
		{"", "Y00", "Xzzz", false},
		{"bzz", "", "c000", false},
		{"a0", "a0V", "a0G", false},
		{"a0", "a0G", "a08", false},
		{"b125", "b129", "b127", false},
		{"a0", "a1V", "a1", false},
		{"Zz", "a01", "a0", false},
		{"", "a0V", "a0", false},
		{"", "b999", "b99", false},
		{"az", "", "b00", false},
		{"", "A00000000000000000000000000", "", true},
		{"", "A000000000000000000000000001", "A000000000000000000000000000V", false},
		{"zzzzzzzzzzzzzzzzzzzzzzzzzzy", "", "zzzzzzzzzzzzzzzzzzzzzzzzzzz", false},
		{"zzzzzzzzzzzzzzzzzzzzzzzzzzz", "", "zzzzzzzzzzzzzzzzzzzzzzzzzzzV", false},
		{"a00", "", "", true},
		{"a00", "a1", "", true},
		{"0", "1", "", true},
		{"a1", "a0", "", true},
	} {
		got, err := KeyBetween(c.a, c.b)
		if c.fails {
			if err == nil {
				t.Errorf("KeyBetween(%q, %q) = %q, want error", c.a, c.b, got)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("KeyBetween(%q, %q) = %q, %v, want %q", c.a, c.b, got, err, c.want)
		}
	}
}

func TestKeysAfterIncrease(t *testing.T) {
	keys, err := KeysAfter("", 200)
	if err != nil {
		t.Fatal(err)
	}
	if keys[0] != "a0" || keys[61] != "az" || keys[62] != "b00" {
		t.Fatalf("keys %v %v %v", keys[0], keys[61], keys[62])
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] || !ValidOrderKey(keys[i]) {
			t.Fatalf("key %d %q after %q", i, keys[i], keys[i-1])
		}
	}
}

func TestValidOrderKey(t *testing.T) {
	for key, want := range map[string]bool{
		"a0": true, "a0V": true, "Zz": true, "b00": true,
		"": false, "a": false, "a00": false, "a0-": false, "0": false, "A00000000000000000000000000": false,
	} {
		if got := ValidOrderKey(key); got != want {
			t.Errorf("ValidOrderKey(%q) = %v", key, got)
		}
	}
}
