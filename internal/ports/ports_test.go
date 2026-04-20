package ports

import "testing"

func TestForAliasDeterministic(t *testing.T) {
	a := ForAlias("auth", 3001, 3999)
	b := ForAlias("auth", 3001, 3999)
	if a != b {
		t.Fatalf("ForAlias not deterministic: %d vs %d", a, b)
	}
}

func TestForAliasInRange(t *testing.T) {
	aliases := []string{"a", "b", "c", "feature-auth", "fix/bug-1", "longer-alias-name-test", "x"}
	for _, alias := range aliases {
		p := ForAlias(alias, 3001, 3999)
		if p < 3001 || p > 3999 {
			t.Errorf("alias %q port %d out of range [3001,3999]", alias, p)
		}
	}
}

func TestForAliasCustomRange(t *testing.T) {
	p := ForAlias("x", 4000, 4010)
	if p < 4000 || p > 4010 {
		t.Errorf("port %d out of [4000,4010]", p)
	}
}

func TestAllocateNoCollision(t *testing.T) {
	used := map[int]bool{}
	got, err := Allocate("auth", 3001, 3999, used)
	if err != nil {
		t.Fatal(err)
	}
	if got != ForAlias("auth", 3001, 3999) {
		t.Errorf("no collision but Allocate != ForAlias: got %d want %d", got, ForAlias("auth", 3001, 3999))
	}
}

func TestAllocateProbes(t *testing.T) {
	base := ForAlias("auth", 3001, 3999)
	used := map[int]bool{base: true, base + 1: true}
	got, err := Allocate("auth", 3001, 3999, used)
	if err != nil {
		t.Fatal(err)
	}
	if got == base || got == base+1 {
		t.Errorf("Allocate returned taken port %d", got)
	}
	if got < 3001 || got > 3999 {
		t.Errorf("Allocate out of range: %d", got)
	}
}

func TestAllocateWrapsAround(t *testing.T) {
	used := map[int]bool{}
	for p := 3001; p <= 3998; p++ {
		used[p] = true
	}
	got, err := Allocate("auth", 3001, 3999, used)
	if err != nil {
		t.Fatal(err)
	}
	if got != 3999 {
		t.Errorf("expected 3999, got %d", got)
	}
}

func TestAllocateExhausted(t *testing.T) {
	used := map[int]bool{}
	for p := 3001; p <= 3999; p++ {
		used[p] = true
	}
	_, err := Allocate("auth", 3001, 3999, used)
	if err == nil {
		t.Fatal("expected error on exhausted range")
	}
}

func TestAllocateInvalidRange(t *testing.T) {
	if _, err := Allocate("auth", 3999, 3001, map[int]bool{}); err == nil {
		t.Fatal("expected error on invalid range (min>max)")
	}
	if _, err := Allocate("auth", 0, 0, map[int]bool{}); err == nil {
		t.Fatal("expected error on zero range")
	}
}
