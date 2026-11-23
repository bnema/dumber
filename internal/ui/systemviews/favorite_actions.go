package systemviews

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
)

const (
	favoriteActionCreate         = "favorite.create"
	favoriteActionSave           = "favorite.save"
	favoriteActionDelete         = "favorite.delete"
	favoriteActionFilterTag      = "favorite.filterTag"
	favoriteActionFilterUntagged = "favorite.filterUntagged"
	favoriteActionClearFilters   = "favorite.clearFilters"
	tagActionCreate              = "tag.create"
	tagActionUpdate              = "tag.update"
	tagActionDelete              = "tag.delete"
)

// favoritesSavedToastTimeout keeps a plain confirmation briefly on screen.
const favoritesSavedToastTimeout = 3 * time.Second

// Favorites regions swapped in place by filters, undo and errors.
const (
	favoritesRegionAlerts = "favorites-alerts"
	favoritesRegionNav    = "favorites-nav"
	favoritesRegionList   = "favorites-list"
)

// favoriteFilterAction reports actions that only change which favorites are
// shown; they swap the nav and list regions instead of remounting.
func favoriteFilterAction(action string) bool {
	switch action {
	case favoriteActionFilterTag, favoriteActionFilterUntagged, favoriteActionClearFilters:
		return true
	default:
		return false
	}
}

// applyFavoritesActionLocked runs a favorites action and refreshes the page.
// Filters swap regions; an error with nothing written swaps only the alert
// region so open edit panels keep their values; success remounts and shows
// a toast. Caller holds the state lock.
func (a *App) applyFavoritesActionLocked(ctx context.Context, event DOMAction) error {
	a.favoritesError = ""
	if favoriteFilterAction(event.Action) {
		a.applyFavoriteFilter(event)
		return a.refreshFavoritesRegionsLocked(ctx, true)
	}
	var (
		wrote bool
		err   error
	)
	// favorite.save reports partial writes, so it is dispatched here
	// rather than through handleFavoriteAction.
	if event.Action == favoriteActionSave && a.deps.Favorites != nil {
		wrote, err = a.saveFavorite(ctx, event.Data)
	} else {
		err = a.handleFavoriteAction(ctx, event)
	}
	if err != nil {
		a.favoritesNotice = ""
		a.favoritesError = err.Error()
		// Nothing written: keep open panels as the user left them.
		if _, ok := a.deps.DOM.(DOMRegionUpdater); ok && !wrote {
			return a.refreshFavoritesRegionsLocked(ctx, false, favoritesRegionAlerts)
		}
		return a.remountFavoritesLocked(ctx)
	}

	notice := a.favoritesNotice
	toaster, canToast := a.deps.DOM.(DOMToaster)
	if canToast && notice != "" {
		a.favoritesNotice = ""
	}
	if err := a.remountFavoritesLocked(ctx); err != nil {
		return err
	}
	if canToast && notice != "" && !toaster.ShowToast(ToastRequest{Message: notice, Timeout: favoritesSavedToastTimeout}) {
		a.favoritesNotice = notice
		return a.remountFavoritesLocked(ctx)
	}
	return nil
}

func (a *App) remountFavoritesLocked(ctx context.Context) error {
	if err := a.loadFavoritesRoute(ctx); err != nil {
		a.renderRouteError(err)
		_ = a.mountRenderedHTML()
		return err
	}
	return a.mountRenderedHTML()
}

// refreshFavoritesRegionsLocked reloads the favorites and swaps the named
// regions (all of them when none is given).
func (a *App) refreshFavoritesRegionsLocked(ctx context.Context, transition bool, names ...string) error {
	if a.currentRoute != RouteFavorites {
		return nil
	}
	if err := a.loadFavoritesRoute(ctx); err != nil {
		a.renderRouteError(err)
		_ = a.mountRenderedHTML()
		return err
	}
	updater, ok := a.deps.DOM.(DOMRegionUpdater)
	if !ok {
		return a.mountRenderedHTML()
	}
	regions := favoritesRegions(a.favoritesData)
	if len(names) > 0 {
		regions = pickRegions(regions, names)
	}
	return updater.UpdateRegions(RegionUpdate{Regions: regions, Transition: transition})
}

func pickRegions(regions []DOMRegion, names []string) []DOMRegion {
	picked := make([]DOMRegion, 0, len(names))
	for _, region := range regions {
		for _, name := range names {
			if region.Name == name {
				picked = append(picked, region)
			}
		}
	}
	return picked
}

func (a *App) applyFavoriteFilter(event DOMAction) {
	a.favoritesNotice = ""
	switch event.Action {
	case favoriteActionFilterTag:
		tagID, err := parsePositiveInt64(event.Data["tagId"], "tag id")
		if err != nil {
			a.favoritesError = err.Error()
			return
		}
		id := entity.TagID(tagID)
		a.favoriteTagFilter = &id
		a.favoriteUntaggedFilter = false
	case favoriteActionFilterUntagged:
		a.favoriteTagFilter = nil
		a.favoriteUntaggedFilter = true
	default:
		a.favoriteTagFilter = nil
		a.favoriteUntaggedFilter = false
	}
}

func (a *App) handleFavoriteAction(ctx context.Context, event DOMAction) error {
	if a.deps.Favorites == nil {
		return fmt.Errorf("favorites service not configured")
	}
	switch event.Action {
	case favoriteActionCreate:
		return a.createFavorite(ctx, event.Data)
	case favoriteActionDelete:
		return a.deleteFavoriteNow(ctx, event.Data)
	case tagActionCreate:
		return a.createTag(ctx, event.Data)
	case tagActionUpdate:
		return a.updateTag(ctx, event.Data)
	case tagActionDelete:
		return a.deleteTagNow(ctx, event.Data)
	default:
		return fmt.Errorf("unknown favorite action: %q", event.Action)
	}
}

func (a *App) createFavorite(ctx context.Context, data map[string]string) error {
	favoriteURL, err := validateFavoriteURL(data["url"])
	if err != nil {
		return err
	}
	tags, err := optionalTagIDs(data["tags"])
	if err != nil {
		return err
	}
	favorite, err := a.deps.Favorites.CreateFavorite(ctx, dto.FavoriteCreateInput{
		URL:   favoriteURL,
		Title: strings.TrimSpace(data["title"]),
		Tags:  tags,
	})
	if err != nil {
		return err
	}
	a.favoritesNotice = "Added " + quotedLabel(favoriteItemLabel(favorite))
	return nil
}

// saveFavorite applies one edit panel: title, icon, shortcut and tags in a
// single submit. An absent field keeps its stored value; a shortcut held by
// another favorite moves to this one. wrote reports whether anything was
// stored before an error, so the caller re-renders from storage.
func (a *App) saveFavorite(ctx context.Context, data map[string]string) (wrote bool, err error) {
	id, err := parsePositiveInt64(data["id"], "favorite id")
	if err != nil {
		return false, err
	}
	favorites, err := a.deps.Favorites.List(ctx)
	if err != nil {
		return false, err
	}
	current, err := findFavorite(favorites, entity.FavoriteID(id))
	if err != nil {
		return false, err
	}

	title := strings.TrimSpace(data["title"])
	if _, ok := data["title"]; !ok {
		title = current.Title
	}
	if title == "" {
		return false, fmt.Errorf("title is required")
	}
	faviconURL := current.FaviconURL
	if raw, ok := data["favicon_url"]; ok {
		faviconURL = strings.TrimSpace(raw)
	}
	input := dto.FavoriteUpdateInput{ID: current.ID, Title: title, FaviconURL: faviconURL}
	if raw, ok := data["shortcut_key"]; ok {
		shortcut, shortcutErr := optionalShortcut(lastFormValue(raw))
		if shortcutErr != nil {
			return false, shortcutErr
		}
		input.ShortcutKey = shortcut
		input.ShortcutKeySet = true
	}
	var assign, remove []entity.TagID
	if raw, ok := data["tags"]; ok {
		wanted, tagErr := optionalTagIDs(raw)
		if tagErr != nil {
			return false, tagErr
		}
		assign, remove = a.favoriteTagDiff(current, wanted)
	}

	if input.ShortcutKeySet && input.ShortcutKey != nil && !sameShortcut(current.ShortcutKey, input.ShortcutKey) {
		released, releaseErr := a.releaseShortcut(ctx, favorites, current.ID, *input.ShortcutKey)
		if releaseErr != nil {
			return partialSaveResult(released, releaseErr)
		}
		wrote = released
	}
	saved, err := a.deps.Favorites.UpdateFavorite(ctx, input)
	if err != nil {
		return partialSaveResult(wrote, err)
	}
	for _, tagID := range assign {
		if assignErr := a.deps.Favorites.AssignTag(ctx, int64(current.ID), int64(tagID)); assignErr != nil {
			return partialSaveResult(true, assignErr)
		}
	}
	for _, tagID := range remove {
		if removeErr := a.deps.Favorites.RemoveTag(ctx, int64(current.ID), int64(tagID)); removeErr != nil {
			return partialSaveResult(true, removeErr)
		}
	}
	a.favoritesNotice = "Saved " + quotedLabel(favoriteItemLabel(saved))
	return true, nil
}

// partialSaveResult wraps err when part of a save was already stored.
func partialSaveResult(wrote bool, err error) (bool, error) {
	if !wrote {
		return false, err
	}
	return true, fmt.Errorf("saved part of the changes; %w", err)
}

func findFavorite(favorites []*entity.Favorite, id entity.FavoriteID) (*entity.Favorite, error) {
	for _, favorite := range favorites {
		if favorite != nil && favorite.ID == id {
			return favorite, nil
		}
	}
	return nil, fmt.Errorf("favorite %d not found", id)
}

// favoriteTagDiff compares the checked tags with the stored ones. Tags
// waiting for their undo window are left alone either way, so a save during
// that window cannot make the undo lose an assignment.
func (a *App) favoriteTagDiff(current *entity.Favorite, wanted []entity.TagID) (assign, remove []entity.TagID) {
	want := make(map[entity.TagID]struct{}, len(wanted))
	for _, id := range wanted {
		if _, pending := a.pendingTagDeletes[int64(id)]; pending {
			continue
		}
		want[id] = struct{}{}
		if !current.HasTag(id) {
			assign = append(assign, id)
		}
	}
	for _, tag := range current.Tags {
		if _, pending := a.pendingTagDeletes[int64(tag.ID)]; pending {
			continue
		}
		if _, keep := want[tag.ID]; !keep {
			remove = append(remove, tag.ID)
		}
	}
	return assign, remove
}

// releaseShortcut clears key from whichever other favorite holds it, since
// a shortcut belongs to one favorite at a time. favorites is the list the
// caller already loaded. wrote reports whether a shortcut was cleared.
func (a *App) releaseShortcut(ctx context.Context, favorites []*entity.Favorite, owner entity.FavoriteID, key int) (wrote bool, err error) {
	for _, favorite := range favorites {
		if favorite == nil || favorite.ID == owner || favorite.ShortcutKey == nil || *favorite.ShortcutKey != key {
			continue
		}
		if err := a.deps.Favorites.SetShortcut(ctx, int64(favorite.ID), nil); err != nil {
			return wrote, err
		}
		wrote = true
	}
	return wrote, nil
}

func sameShortcut(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (a *App) deleteFavoriteNow(ctx context.Context, data map[string]string) error {
	id, err := parsePositiveInt64(data["id"], "favorite id")
	if err != nil {
		return err
	}
	if err := a.deps.Favorites.DeleteFavorite(ctx, id); err != nil {
		return err
	}
	a.favoritesNotice = "Deleted favorite"
	return nil
}

func (a *App) createTag(ctx context.Context, data map[string]string) error {
	name := strings.TrimSpace(data["name"])
	if name == "" {
		return fmt.Errorf("tag name is required")
	}
	color, err := tagColorInput(data)
	if err != nil {
		return err
	}
	tag, err := a.deps.Favorites.CreateTag(ctx, name, color)
	if err != nil {
		return err
	}
	a.favoritesNotice = "Created tag " + quotedLabel(tag.Name)
	return nil
}

func (a *App) updateTag(ctx context.Context, data map[string]string) error {
	id, err := parsePositiveInt64(data["id"], "tag id")
	if err != nil {
		return err
	}
	name := strings.TrimSpace(data["name"])
	if name == "" {
		return fmt.Errorf("tag name is required")
	}
	color, err := tagColorInput(data)
	if err != nil {
		return err
	}
	if err := a.deps.Favorites.UpdateTag(ctx, id, name, color); err != nil {
		return err
	}
	a.favoritesNotice = "Saved tag " + quotedLabel(name)
	return nil
}

func (a *App) deleteTagNow(ctx context.Context, data map[string]string) error {
	id, err := parsePositiveInt64(data["id"], "tag id")
	if err != nil {
		return err
	}
	if err := a.deps.Favorites.DeleteTag(ctx, id); err != nil {
		return err
	}
	if a.favoriteTagFilter != nil && int64(*a.favoriteTagFilter) == id {
		a.favoriteTagFilter = nil
	}
	a.favoritesNotice = "Deleted tag"
	return nil
}

// tagColorInput reads the tag color: the swatch radios and the free color
// picker share the name "color"; the last checked value wins. Empty keeps
// the service default.
func tagColorInput(data map[string]string) (string, error) {
	color := strings.TrimSpace(lastFormValue(data["color"]))
	if color == "" {
		return "", nil
	}
	if safeTagColor(color) != color {
		return "", fmt.Errorf("tag color must be a hex color like #3b82f6")
	}
	return strings.ToLower(color), nil
}

// lastFormValue returns the last comma-separated value of a field sent more
// than once (a hidden default followed by the user's choice).
func lastFormValue(raw string) string {
	if i := strings.LastIndex(raw, ","); i >= 0 {
		return raw[i+1:]
	}
	return raw
}

func quotedLabel(label string) string {
	const maxRunes = 48
	if runes := []rune(label); len(runes) > maxRunes {
		label = string(runes[:maxRunes-1]) + "…"
	}
	return "“" + label + "”"
}

func validateFavoriteURL(raw string) (string, error) {
	favoriteURL := strings.TrimSpace(raw)
	if favoriteURL == "" {
		return "", fmt.Errorf("favorite URL is required")
	}
	parsed, err := url.Parse(favoriteURL)
	if err != nil || parsed.Scheme == "" {
		return "", fmt.Errorf("favorite URL must be absolute")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		if parsed.Host == "" {
			return "", fmt.Errorf("favorite URL host is required")
		}
	case "dumb":
		if routeFromPageHost(parsed.Host, parsed.Opaque) != RouteUnknown {
			return favoriteURL, nil
		}
		return "", fmt.Errorf("favorite URL must target a known systemview route")
	default:
		return "", fmt.Errorf("favorite URL must use http, https, or dumb scheme")
	}
	return favoriteURL, nil
}

func parsePositiveInt64(raw, label string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", label, err)
	}
	if id <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", label)
	}
	return id, nil
}

// optionalTagIDs parses a comma-separated tag id list. Empty entries (the
// hidden field that marks the list as sent) are skipped.
func optionalTagIDs(raw string) ([]entity.TagID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ",")
	ids := make([]entity.TagID, 0, len(parts))
	seen := make(map[entity.TagID]struct{}, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		id, err := parsePositiveInt64(part, "tag id")
		if err != nil {
			return nil, err
		}
		tagID := entity.TagID(id)
		if _, ok := seen[tagID]; ok {
			continue
		}
		seen[tagID] = struct{}{}
		ids = append(ids, tagID)
	}
	return ids, nil
}

// optionalShortcut accepts favorite keyboard shortcuts 1–9; empty input clears the shortcut.
func optionalShortcut(raw string) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > 9 {
		return nil, fmt.Errorf("shortcut key must be 1-9")
	}
	return &value, nil
}
