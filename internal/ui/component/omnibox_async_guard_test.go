package component

import "testing"

func TestOmnibox_IsSearchTokenCurrent(t *testing.T) {
	o := &Omnibox{}
	o.visible = true
	o.searchToken = 3

	if !o.isSearchTokenCurrent(3) {
		t.Fatalf("expected token to be current")
	}
	if o.isSearchTokenCurrent(2) {
		t.Fatalf("expected stale token to be rejected")
	}

	o.visible = false
	if o.isSearchTokenCurrent(3) {
		t.Fatalf("expected hidden omnibox to reject token")
	}
}

func TestOmnibox_IsGhostTokenCurrent(t *testing.T) {
	o := &Omnibox{}
	o.visible = true
	o.ghostToken = 5
	o.realInput = "git"

	if !o.isGhostTokenCurrent(5, "git") {
		t.Fatalf("expected ghost token to be current")
	}
	if o.isGhostTokenCurrent(4, "git") {
		t.Fatalf("expected stale ghost token to be rejected")
	}
	if o.isGhostTokenCurrent(5, "github") {
		t.Fatalf("expected query mismatch to be rejected")
	}
}

func TestOmnibox_IsBangUpdateCurrent(t *testing.T) {
	o := &Omnibox{}
	o.visible = true
	o.searchToken = 9
	o.realInput = "!gh"

	if !o.isBangUpdateCurrent("!gh", 9) {
		t.Fatalf("expected bang update to be current")
	}
	if o.isBangUpdateCurrent("!g", 9) {
		t.Fatalf("expected bang query mismatch to be rejected")
	}
	if o.isBangUpdateCurrent("!gh", 8) {
		t.Fatalf("expected stale bang token to be rejected")
	}

	o.visible = false
	if o.isBangUpdateCurrent("!gh", 9) {
		t.Fatalf("expected hidden omnibox to reject bang updates")
	}
}
