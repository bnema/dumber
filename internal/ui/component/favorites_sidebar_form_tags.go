package component

import (
	"strings"

	"github.com/bnema/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/domain/entity"
)

func favoriteTags(fav *entity.Favorite) []entity.Tag {
	if fav == nil {
		return nil
	}
	return fav.Tags
}

func tagIDSet(tags []entity.Tag) map[entity.TagID]struct{} {
	ids := make(map[entity.TagID]struct{}, len(tags))
	for _, tag := range tags {
		ids[tag.ID] = struct{}{}
	}
	return ids
}

func (fs *FavoritesSidebar) setupFormTagSearch() {
	if fs == nil || fs.formTagSearch == nil {
		return
	}
	changed := func(_ gtk.SearchEntry) { fs.renderFormTagMatches(fs.formTagSearch.GetText()) }
	fs.formCallbacks = append(fs.formCallbacks, changed)
	fs.formTagSearch.ConnectSearchChanged(&changed)
}

func formTagCandidates(tags []*entity.Tag, selected map[entity.TagID]struct{}, query string) []*entity.Tag {
	query = strings.ToLower(strings.TrimSpace(query))
	matches := make([]*entity.Tag, 0, len(tags))
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		if query == "" {
			if _, ok := selected[tag.ID]; ok {
				matches = append(matches, tag)
			}
			continue
		}
		if strings.Contains(strings.ToLower(tag.Name), query) {
			matches = append(matches, tag)
		}
	}
	return matches
}

func (fs *FavoritesSidebar) renderFormTagMatches(query string) {
	if fs == nil || fs.formTagMatches == nil {
		return
	}
	fs.mu.RLock()
	tags := append([]*entity.Tag(nil), fs.allTags...)
	selected := make(map[entity.TagID]struct{}, len(fs.formTagIDs))
	for id := range fs.formTagIDs {
		selected[id] = struct{}{}
	}
	fs.mu.RUnlock()

	clearBoxChildren(fs.formTagMatches)
	callbacks := make([]any, 0)
	for _, tag := range formTagCandidates(tags, selected, query) {
		t := *tag
		label := t.Name
		if _, ok := selected[t.ID]; ok {
			label = "✓ " + label
		}
		button := gtk.NewButtonWithLabel(label)
		if button == nil {
			continue
		}
		button.AddCssClass("favorites-sidebar-form-tag-match")
		callback := func(_ gtk.Button) { fs.toggleFormTag(t.ID) }
		callbacks = append(callbacks, callback)
		button.ConnectClicked(&callback)
		fs.formTagMatches.Append(&button.Widget)
	}
	fs.mu.Lock()
	if !fs.destroyed {
		fs.formTagMatchCallbacks = callbacks
	}
	fs.mu.Unlock()
}

func (fs *FavoritesSidebar) toggleFormTag(tagID entity.TagID) {
	if fs == nil {
		return
	}
	fs.mu.Lock()
	if fs.destroyed {
		fs.mu.Unlock()
		return
	}
	if fs.formTagIDs == nil {
		fs.formTagIDs = make(map[entity.TagID]struct{})
	}
	if _, ok := fs.formTagIDs[tagID]; ok {
		delete(fs.formTagIDs, tagID)
	} else {
		fs.formTagIDs[tagID] = struct{}{}
	}
	fs.mu.Unlock()
	query := ""
	if fs.formTagSearch != nil {
		query = fs.formTagSearch.GetText()
	}
	fs.renderFormTagMatches(query)
}

func (fs *FavoritesSidebar) formTagIDsSnapshot() []entity.TagID {
	if fs == nil {
		return nil
	}
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	ids := make([]entity.TagID, 0, len(fs.formTagIDs))
	for id := range fs.formTagIDs {
		ids = append(ids, id)
	}
	return ids
}

func (fs *FavoritesSidebar) updateFavoriteTags(favoriteID entity.FavoriteID, wanted []entity.TagID) bool {
	if fs == nil {
		return false
	}
	fs.mu.RLock()
	uc, ctx := fs.favoritesUC, fs.ctx
	fs.mu.RUnlock()
	if uc == nil {
		return false
	}
	if err := uc.UpdateFavoriteTags(ctx, favoriteID, wanted); err != nil {
		fs.startLoad()
		fs.setNotice(err.Error())
		return false
	}
	return true
}
