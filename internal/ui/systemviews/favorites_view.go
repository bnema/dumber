package systemviews

import (
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/domain/entity"
)

const defaultTagColor = "#808080"

// tagColor is one swatch offered when creating or editing a tag.
type tagColor struct {
	Value string
	Name  string
}

var tagColorPresets = []tagColor{
	{"#808080", "Gray"}, {"#ef4444", "Red"}, {"#f97316", "Orange"},
	{"#eab308", "Yellow"}, {"#22c55e", "Green"}, {"#14b8a6", "Teal"},
	{"#3b82f6", "Blue"}, {"#8b5cf6", "Violet"}, {"#ec4899", "Pink"},
}

const favoriteShortcutCount = 9

type favoritesRenderData struct {
	// Favorites are the visible favorites: filtered, without pending
	// deletes, and with pending tags removed from their tag lists.
	Favorites []*entity.Favorite
	Tags      []*entity.Tag
	// TagCounts counts every (unfiltered) favorite per tag.
	TagCounts     map[entity.TagID]int
	TotalCount    int
	UntaggedCount int
	// ShortcutOwners maps a shortcut key to the favorite holding it.
	ShortcutOwners map[int]*entity.Favorite
	TagFilter      *entity.TagID
	UntaggedFilter bool
	Notice         string
	Error          string
}

// favoritesSource is what the favorites view is built from.
type favoritesSource struct {
	Favorites       []*entity.Favorite
	Tags            []*entity.Tag
	HiddenFavorites map[int64]string
	HiddenTags      map[int64]string
	TagFilter       *entity.TagID
	UntaggedFilter  bool
}

// buildFavoritesRenderData hides pending deletes, counts favorites per tag
// and applies the active filter. A filter on a tag that no longer exists
// falls back to All.
func buildFavoritesRenderData(src favoritesSource) favoritesRenderData {
	data := favoritesRenderData{
		TagCounts:      map[entity.TagID]int{},
		ShortcutOwners: map[int]*entity.Favorite{},
		UntaggedFilter: src.UntaggedFilter,
	}
	for _, tag := range src.Tags {
		if tag == nil {
			continue
		}
		if _, hidden := src.HiddenTags[int64(tag.ID)]; hidden {
			continue
		}
		data.Tags = append(data.Tags, tag)
		if src.TagFilter != nil && *src.TagFilter == tag.ID {
			id := tag.ID
			data.TagFilter = &id
		}
	}

	visible := make([]*entity.Favorite, 0, len(src.Favorites))
	for _, favorite := range src.Favorites {
		if favorite == nil {
			continue
		}
		if _, hidden := src.HiddenFavorites[int64(favorite.ID)]; hidden {
			continue
		}
		favorite = withoutHiddenTags(favorite, src.HiddenTags)
		visible = append(visible, favorite)
		data.TotalCount++
		if len(favorite.Tags) == 0 {
			data.UntaggedCount++
		}
		for _, tag := range favorite.Tags {
			data.TagCounts[tag.ID]++
		}
		if favorite.ShortcutKey != nil {
			data.ShortcutOwners[*favorite.ShortcutKey] = favorite
		}
	}
	data.Favorites = filterFavorites(visible, data.TagFilter, data.UntaggedFilter)
	return data
}

func withoutHiddenTags(favorite *entity.Favorite, hidden map[int64]string) *entity.Favorite {
	if len(hidden) == 0 || len(favorite.Tags) == 0 {
		return favorite
	}
	kept := make([]entity.Tag, 0, len(favorite.Tags))
	for _, tag := range favorite.Tags {
		if _, skip := hidden[int64(tag.ID)]; !skip {
			kept = append(kept, tag)
		}
	}
	if len(kept) == len(favorite.Tags) {
		return favorite
	}
	clone := *favorite
	clone.Tags = kept
	return &clone
}

func favoritesHTML(data favoritesRenderData) string {
	return mustRenderComponent(FavoritesView(data))
}

// favoritesRegions renders the parts of the page swapped in place.
func favoritesRegions(data favoritesRenderData) []DOMRegion {
	return []DOMRegion{
		{Name: favoritesRegionAlerts, HTML: mustRenderComponent(FavoritesAlerts(data))},
		{Name: favoritesRegionNav, HTML: mustRenderComponent(FavoritesNav(data))},
		{Name: favoritesRegionList, HTML: mustRenderComponent(FavoritesList(data))},
	}
}

func favoritesDocumentTitle(data favoritesRenderData) string {
	count := len(data.Favorites)
	if data.TagFilter != nil || data.UntaggedFilter {
		if count == 1 {
			return "Favorites — 1 filtered bookmark"
		}
		return fmt.Sprintf("Favorites — %d filtered bookmarks", count)
	}
	if count == 1 {
		return "Favorites — 1 bookmark"
	}
	return fmt.Sprintf("Favorites — %d bookmarks", count)
}

func favoritesTitleDetail(data favoritesRenderData) string {
	if data.TotalCount == 0 {
		return ""
	}
	return fmt.Sprintf("%d", data.TotalCount)
}

// favoritesFilterLabel names the active filter, for the list heading.
func favoritesFilterLabel(data favoritesRenderData) string {
	if data.UntaggedFilter {
		return "Untagged"
	}
	if tag := activeFilterTag(data); tag != nil {
		return tag.Name
	}
	return "All favorites"
}

func activeFilterTag(data favoritesRenderData) *entity.Tag {
	if data.TagFilter == nil {
		return nil
	}
	for _, tag := range data.Tags {
		if tag.ID == *data.TagFilter {
			return tag
		}
	}
	return nil
}

// favoritesCreateTagValue preselects the filtered tag for new favorites.
func favoritesCreateTagValue(data favoritesRenderData) string {
	if data.TagFilter == nil {
		return ""
	}
	return fmt.Sprintf("%d", *data.TagFilter)
}

func favoritesEmptyMessage(data favoritesRenderData) string {
	switch {
	case data.TotalCount == 0:
		return "No favorites yet"
	case data.UntaggedFilter:
		return "Every favorite has a tag"
	default:
		return "No favorites with this tag"
	}
}

func favoritesEmptyHint(data favoritesRenderData) string {
	if data.TotalCount == 0 {
		return "Paste a URL above, or press Ctrl+D on any page."
	}
	return "Open a favorite and tick this tag, or add one above."
}

func countLabel(n int) string {
	return fmt.Sprintf("%d", n)
}

func favoriteFilterActive(data favoritesRenderData, kind string, tagID entity.TagID) bool {
	switch kind {
	case "all":
		return data.TagFilter == nil && !data.UntaggedFilter
	case "untagged":
		return data.UntaggedFilter
	default:
		return data.TagFilter != nil && *data.TagFilter == tagID
	}
}

// ariaCurrent marks the active filter for assistive technology.
func ariaCurrent(active bool) string {
	if active {
		return "page"
	}
	return "false"
}

func favoriteItemLabel(favorite *entity.Favorite) string {
	if favorite == nil {
		return ""
	}
	label := strings.TrimSpace(favorite.Title)
	if label == "" {
		label = favorite.URL
	}
	return label
}

func favoriteDisplayURL(favorite *entity.Favorite) string {
	return stripURLScheme(favorite.URL)
}

func favoriteFaviconURL(favorite *entity.Favorite) string {
	return historyItemFaviconURL(&entity.HistoryEntry{URL: favorite.URL})
}

func favoriteFaviconFallback(favorite *entity.Favorite) string {
	label := strings.TrimSpace(favoriteItemLabel(favorite))
	label = stripURLScheme(label)
	for _, r := range label {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// favoriteSearchText feeds the local text filter.
func favoriteSearchText(favorite *entity.Favorite) string {
	parts := []string{favorite.Title, favorite.URL}
	for _, tag := range favorite.Tags {
		parts = append(parts, tag.Name)
	}
	return strings.ToLower(strings.Join(parts, " "))
}

// favoriteTags resolves a favorite's tags against the tag list, so chips use
// the current name and color.
func favoriteTags(favorite *entity.Favorite, tags []*entity.Tag) []*entity.Tag {
	out := make([]*entity.Tag, 0, len(favorite.Tags))
	for _, tag := range tags {
		if favorite.HasTag(tag.ID) {
			out = append(out, tag)
		}
	}
	return out
}

func favoriteDOMID(favorite *entity.Favorite, part string) string {
	return fmt.Sprintf("sv-fav-%d-%s", favorite.ID, part)
}

func tagDOMID(tag *entity.Tag, part string) string {
	return fmt.Sprintf("sv-tag-%d-%s", tag.ID, part)
}

func shortcutChecked(selected *int, value int) bool {
	if value == 0 {
		return selected == nil
	}
	return selected != nil && *selected == value
}

func shortcutValue(value int) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprintf("%d", value)
}

func shortcutOptionLabel(value int) string {
	if value == 0 {
		return "None"
	}
	return fmt.Sprintf("%d", value)
}

func shortcutOptions() []int {
	options := make([]int, 0, favoriteShortcutCount+1)
	options = append(options, 0)
	for i := 1; i <= favoriteShortcutCount; i++ {
		options = append(options, i)
	}
	return options
}

// shortcutOwnerNote explains that picking a key held elsewhere moves it.
func shortcutOwnerNote(data favoritesRenderData, favorite *entity.Favorite, value int) string {
	owner, ok := data.ShortcutOwners[value]
	if !ok || value == 0 || owner.ID == favorite.ID {
		return ""
	}
	return "Used by " + favoriteItemLabel(owner) + "; choosing it moves the shortcut here"
}

func shortcutBadge(favorite *entity.Favorite) string {
	if favorite.ShortcutKey == nil {
		return ""
	}
	return fmt.Sprintf("%d", *favorite.ShortcutKey)
}

func tagColorIsPreset(color string) bool {
	color = strings.ToLower(safeTagColor(color))
	for _, preset := range tagColorPresets {
		if preset.Value == color {
			return true
		}
	}
	return false
}

// tagCustomColor is the value of the "custom" swatch: the tag color when it
// is not a preset, or a neutral default.
func tagCustomColor(color string) string {
	if color != "" && !tagColorIsPreset(color) {
		return colorPickerValue(safeTagColor(color))
	}
	return "#64748b"
}

func tagColorChecked(current, preset string) bool {
	return strings.EqualFold(safeTagColor(current), preset)
}

func tagDotStyle(color string) string {
	return "--tag-color:" + safeTagColor(color)
}

func safeTagColor(raw string) string {
	value := strings.TrimSpace(raw)
	if len(value) != 4 && len(value) != 7 {
		return defaultTagColor
	}
	if value[0] != '#' {
		return defaultTagColor
	}
	for _, ch := range value[1:] {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F') {
			continue
		}
		return defaultTagColor
	}
	return value
}

func filterFavorites(favorites []*entity.Favorite, tagID *entity.TagID, untagged bool) []*entity.Favorite {
	if tagID == nil && !untagged {
		return favorites
	}
	out := make([]*entity.Favorite, 0, len(favorites))
	for _, favorite := range favorites {
		if favorite == nil {
			continue
		}
		if untagged {
			if len(favorite.Tags) != 0 {
				continue
			}
		} else if tagID != nil && !favorite.HasTag(*tagID) {
			continue
		}
		out = append(out, favorite)
	}
	return out
}

func tagFormAction(tag *entity.Tag) string {
	if tag == nil {
		return tagActionCreate
	}
	return tagActionUpdate
}

// nextTagColor suggests the first preset no tag uses yet, so new tags are
// told apart without picking a color.
func nextTagColor(tags []*entity.Tag) string {
	used := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		if tag != nil {
			used[strings.ToLower(safeTagColor(tag.Color))] = struct{}{}
		}
	}
	for _, preset := range tagColorPresets[1:] {
		if _, taken := used[preset.Value]; !taken {
			return preset.Value
		}
	}
	return tagColorPresets[0].Value
}
