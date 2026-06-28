package usecase

import (
	"reflect"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
)

func TestFilterFavoritesForSidebarEmptyQueryReturnsStableOrder(t *testing.T) {
	favorites := []*entity.Favorite{
		favoriteSidebarTestFavorite(1, "https://third.example", "Third", 30),
		favoriteSidebarTestFavorite(2, "https://first.example", "First", 10),
		favoriteSidebarTestFavorite(3, "https://second.example", "Second", 20),
	}

	got := FilterFavoritesForSidebar(favorites, FavoriteSidebarQuery{})

	assertFavoriteSidebarIDs(t, got, []entity.FavoriteID{1, 2, 3})
	if len(got) > 0 && got[0] != favorites[0] {
		t.Fatalf("FilterFavoritesForSidebar should return original favorite pointers in a new slice")
	}
	if len(favorites) != 3 || favorites[0].ID != 1 || favorites[1].ID != 2 || favorites[2].ID != 3 {
		t.Fatalf("FilterFavoritesForSidebar mutated input order: %#v", favorites)
	}
}

func TestFilterFavoritesForSidebarMatchesTitleURLAndTagNames(t *testing.T) {
	favorites := []*entity.Favorite{
		favoriteSidebarTestFavorite(1, "https://docs.example/go", "Go Docs", 1, favoriteSidebarTestTag(1, "reference")),
		favoriteSidebarTestFavorite(2, "https://example.com/articles", "Reading List", 2, favoriteSidebarTestTag(2, "Research")),
		favoriteSidebarTestFavorite(3, "https://calendar.example", "Planner", 3, favoriteSidebarTestTag(3, "Work")),
	}

	t.Run("title", func(t *testing.T) {
		got := FilterFavoritesForSidebar(favorites, FavoriteSidebarQuery{Text: "go"})
		assertFavoriteSidebarIDs(t, got, []entity.FavoriteID{1})
	})

	t.Run("url", func(t *testing.T) {
		got := FilterFavoritesForSidebar(favorites, FavoriteSidebarQuery{Text: "articles"})
		assertFavoriteSidebarIDs(t, got, []entity.FavoriteID{2})
	})

	t.Run("tag", func(t *testing.T) {
		got := FilterFavoritesForSidebar(favorites, FavoriteSidebarQuery{Text: "research"})
		assertFavoriteSidebarIDs(t, got, []entity.FavoriteID{2})
	})
}

func TestFilterFavoritesForSidebarSelectedTagsMatchAny(t *testing.T) {
	favorites := []*entity.Favorite{
		favoriteSidebarTestFavorite(1, "https://go.dev", "Go", 1, favoriteSidebarTestTag(1, "dev")),
		favoriteSidebarTestFavorite(2, "https://recipes.example", "Recipes", 2, favoriteSidebarTestTag(2, "home")),
		favoriteSidebarTestFavorite(3, "https://garden.example", "Garden", 3, favoriteSidebarTestTag(3, "outdoor")),
		favoriteSidebarTestFavorite(4, "https://untagged.example", "Untagged", 4),
	}

	got := FilterFavoritesForSidebar(favorites, FavoriteSidebarQuery{TagIDs: []entity.TagID{1, 3}})

	assertFavoriteSidebarIDs(t, got, []entity.FavoriteID{1, 3})
}

func TestFilterFavoritesForSidebarCombinesTextWithinSelectedTags(t *testing.T) {
	favorites := []*entity.Favorite{
		favoriteSidebarTestFavorite(1, "https://go.dev/doc", "Go Docs", 1, favoriteSidebarTestTag(1, "dev")),
		favoriteSidebarTestFavorite(2, "https://go.example/home", "Go Home", 2, favoriteSidebarTestTag(2, "home")),
		favoriteSidebarTestFavorite(3, "https://rust.dev", "Rust", 3, favoriteSidebarTestTag(1, "dev")),
	}

	got := FilterFavoritesForSidebar(favorites, FavoriteSidebarQuery{Text: "go", TagIDs: []entity.TagID{1}})

	assertFavoriteSidebarIDs(t, got, []entity.FavoriteID{1})
}

func TestFilterFavoritesForSidebarRanksExactAndPrefixMatches(t *testing.T) {
	favorites := []*entity.Favorite{
		favoriteSidebarTestFavorite(1, "https://example.com/go-notes", "My Go Notes", 1),
		favoriteSidebarTestFavorite(2, "https://go.example.com", "Other", 2),
		favoriteSidebarTestFavorite(3, "https://example.com/other", "Go", 3),
		favoriteSidebarTestFavorite(4, "https://example.com/go", "Go Handbook", 4),
	}

	got := FilterFavoritesForSidebar(favorites, FavoriteSidebarQuery{Text: "go"})

	assertFavoriteSidebarIDs(t, got, []entity.FavoriteID{3, 4, 2, 1})
}

func TestFilterFavoritesForSidebarRanksHostPrefixAndTagNameMatches(t *testing.T) {
	favorites := []*entity.Favorite{
		favoriteSidebarTestFavorite(1, "https://example.com/golang", "Notes", 1),
		favoriteSidebarTestFavorite(2, "https://pkg.go.dev", "Packages", 2),
		favoriteSidebarTestFavorite(3, "https://example.org/tools", "Tools", 3, favoriteSidebarTestTag(9, "Go Tools")),
		favoriteSidebarTestFavorite(4, "https://example.net/articles", "Long Go Article", 4),
	}

	got := FilterFavoritesForSidebar(favorites, FavoriteSidebarQuery{Text: "go"})

	assertFavoriteSidebarIDs(t, got, []entity.FavoriteID{2, 3, 4, 1})
}

func TestFilterFavoritesForSidebarDeduplicatesRepeatedPointers(t *testing.T) {
	favorite := favoriteSidebarTestFavorite(1, "https://example.com/one", "One", 1)
	favorites := []*entity.Favorite{
		favorite,
		favoriteSidebarTestFavorite(0, "https://example.com/no-id", "First no ID", 2),
		favorite,
		favoriteSidebarTestFavorite(0, "https://example.com/no-id", "Second no ID", 3),
		nil,
	}

	got := FilterFavoritesForSidebar(favorites, FavoriteSidebarQuery{})

	assertFavoriteSidebarIDs(t, got, []entity.FavoriteID{1, 0})
	if len(got) != 2 || got[1].Title != "First no ID" {
		t.Fatalf("expected duplicate no-ID URL to keep first favorite, got %#v", got)
	}
}

func TestFilterFavoritesForSidebarUsesPositionAndURLTieBreakers(t *testing.T) {
	favorites := []*entity.Favorite{
		favoriteSidebarTestFavorite(1, "https://z.example/go", "Notes about Go", 2),
		favoriteSidebarTestFavorite(2, "https://a.example/go", "More Go Notes", 2),
		favoriteSidebarTestFavorite(3, "https://m.example/go", "Go snippets", 1),
	}

	got := FilterFavoritesForSidebar(favorites, FavoriteSidebarQuery{Text: "go"})

	assertFavoriteSidebarIDs(t, got, []entity.FavoriteID{3, 2, 1})
}

func favoriteSidebarTestFavorite(id entity.FavoriteID, rawURL string, title string, position int, tags ...entity.Tag) *entity.Favorite {
	return &entity.Favorite{
		ID:       id,
		URL:      rawURL,
		Title:    title,
		Position: position,
		Tags:     append([]entity.Tag(nil), tags...),
	}
}

func favoriteSidebarTestTag(id entity.TagID, name string) entity.Tag {
	return entity.Tag{ID: id, Name: name}
}

func assertFavoriteSidebarIDs(t *testing.T, favorites []*entity.Favorite, want []entity.FavoriteID) {
	t.Helper()
	got := make([]entity.FavoriteID, 0, len(favorites))
	for _, favorite := range favorites {
		if favorite == nil {
			t.Fatalf("result contains nil favorite")
		}
		got = append(got, favorite.ID)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("favorite IDs = %v, want %v", got, want)
	}
}
