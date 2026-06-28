package entity

import "testing"

func TestFavoriteSupportsMultipleTagsAndNoTags(t *testing.T) {
	fav := NewFavorite("https://example.com", "Example")
	if fav.HasTag(TagID(1)) {
		t.Fatal("new favorite unexpectedly has tag")
	}

	fav.Tags = []Tag{{ID: 1, Name: "Go"}, {ID: 2, Name: "Docs"}}
	if !fav.HasTag(1) || !fav.HasTag(2) {
		t.Fatalf("favorite tags not detected: %+v", fav.Tags)
	}
	if fav.HasTag(3) {
		t.Fatal("favorite reported missing tag as present")
	}
}

func TestTagStoresTrimmedDisplayNameAndColor(t *testing.T) {
	tag := NewTag("  Read Later  ")
	if tag.Name != "Read Later" {
		t.Fatalf("tag name = %q", tag.Name)
	}
	if tag.Color == "" {
		t.Fatal("tag default color is empty")
	}
}
