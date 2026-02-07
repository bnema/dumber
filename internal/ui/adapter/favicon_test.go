package adapter

import "testing"

func TestFaviconWarningDedup_FirstAndRepeated(t *testing.T) {
	adapter := NewFaviconAdapter(nil, nil)

	first, suppressed := adapter.shouldLogWarningDedup("save-png:example.com")
	if !first {
		t.Fatalf("expected first warning to be logged")
	}
	if suppressed != 0 {
		t.Fatalf("expected no suppressed count on first warning, got %d", suppressed)
	}

	first, suppressed = adapter.shouldLogWarningDedup("save-png:example.com")
	if first {
		t.Fatalf("expected repeated warning to be suppressed")
	}
	if suppressed != 1 {
		t.Fatalf("expected suppressed count to be 1, got %d", suppressed)
	}

	first, suppressed = adapter.shouldLogWarningDedup("save-png:example.com")
	if first {
		t.Fatalf("expected repeated warning to remain suppressed")
	}
	if suppressed != 2 {
		t.Fatalf("expected suppressed count to be 2, got %d", suppressed)
	}
}

func TestFaviconWarningDedup_ClearResetsState(t *testing.T) {
	adapter := NewFaviconAdapter(nil, nil)
	key := "sized-png:example.com"

	first, _ := adapter.shouldLogWarningDedup(key)
	if !first {
		t.Fatalf("expected first warning to be logged")
	}

	adapter.clearWarningDedup(key)

	first, suppressed := adapter.shouldLogWarningDedup(key)
	if !first {
		t.Fatalf("expected warning to log again after clear")
	}
	if suppressed != 0 {
		t.Fatalf("expected suppressed count reset after clear, got %d", suppressed)
	}
}
