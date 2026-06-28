package component

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/application/dto"
	"github.com/bnema/dumber/internal/domain/entity"
)

type favoritesSidebarMode int

const (
	favoritesSidebarModeNone favoritesSidebarMode = iota
	favoritesSidebarModeAdd
	favoritesSidebarModeEdit
	favoritesSidebarModeTag
	favoritesSidebarModeShortcut
)

type favoritesSidebarFocusZone int

const (
	favoritesSidebarFocusSearch favoritesSidebarFocusZone = iota
	favoritesSidebarFocusTags
	favoritesSidebarFocusList
	favoritesSidebarFocusForm
	favoritesSidebarFocusConfirm
)

func (fs *FavoritesSidebar) beginAddForm() {
	fs.mu.Lock()
	if fs.destroyed {
		fs.mu.Unlock()
		return
	}
	fs.mode = favoritesSidebarModeAdd
	fs.editingID = 0
	fs.confirmDelete = false
	fs.notice = "Add favorite: URL, title, comma-separated tag IDs, shortcut 1-9. Press Ctrl+Enter or Save."
	fs.mu.Unlock()
	fs.renderForm(nil)
	fs.rebuildList()
}

func (fs *FavoritesSidebar) beginEditForm() {
	fav := fs.selectedFavorite()
	if fav == nil {
		fs.setNotice("Select a favorite to edit")
		return
	}
	fs.mu.Lock()
	if fs.destroyed {
		fs.mu.Unlock()
		return
	}
	fs.mode = favoritesSidebarModeEdit
	fs.editingID = fav.ID
	fs.confirmDelete = false
	fs.notice = "Edit favorite: URL is read-only; use tag mode for tags. Press Ctrl+Enter or Save."
	fs.mu.Unlock()
	fs.renderForm(fav)
	fs.rebuildList()
}

func (fs *FavoritesSidebar) renderForm(fav *entity.Favorite) {
	if fs == nil || fs.formBox == nil {
		return
	}
	clearBoxChildren(fs.formBox)
	fs.formURLEntry = gtk.NewSearchEntry()
	fs.formTitleEntry = gtk.NewSearchEntry()
	fs.formTagsEntry = nil
	if fav == nil {
		fs.formTagsEntry = gtk.NewSearchEntry()
	}
	fs.formShortcutEntry = gtk.NewSearchEntry()
	entries := []struct {
		entry *gtk.SearchEntry
		label string
	}{
		{entry: fs.formURLEntry, label: "URL"},
		{entry: fs.formTitleEntry, label: "Title"},
		{entry: fs.formTagsEntry, label: "Tag IDs"},
		{entry: fs.formShortcutEntry, label: "Shortcut"},
	}
	for _, item := range entries {
		if item.entry == nil {
			continue
		}
		placeholder := item.label
		item.entry.SetPlaceholderText(&placeholder)
		fs.formBox.Append(&item.entry.Widget)
	}
	fs.formSaveButton = gtk.NewButtonWithLabel("Save")
	if fs.formSaveButton != nil {
		cb := func(_ gtk.Button) {
			fs.submitForm()
		}
		fs.retainedCallbacks = append(fs.retainedCallbacks, cb)
		fs.formSaveButton.ConnectClicked(&cb)
		fs.formBox.Append(&fs.formSaveButton.Widget)
	}
	if fav != nil {
		fs.formURL = fav.URL
		fs.formTitle = fav.Title
		fs.formTags = joinFavoriteTagIDs(fav.Tags)
		fs.formShortcut = ""
		if fav.ShortcutKey != nil {
			fs.formShortcut = strconv.Itoa(*fav.ShortcutKey)
		}
		if fs.formURLEntry != nil {
			fs.formURLEntry.SetText(fs.formURL)
			fs.formURLEntry.SetEditable(false)
		}
		if fs.formTitleEntry != nil {
			fs.formTitleEntry.SetText(fs.formTitle)
		}
		if fs.formTagsEntry != nil {
			fs.formTagsEntry.SetText(fs.formTags)
		}
		if fs.formShortcutEntry != nil {
			fs.formShortcutEntry.SetText(fs.formShortcut)
		}
	} else {
		fs.formURL, fs.formTitle, fs.formTags, fs.formShortcut = "", "", "", ""
	}
	fs.formBox.SetVisible(true)
	fs.focusForm()
}

func (fs *FavoritesSidebar) focusForm() {
	fs.scheduleIdle(glib.SourceFunc(func(uintptr) bool {
		if fs != nil && fs.formURLEntry != nil {
			fs.formURLEntry.GrabFocus()
		}
		return false
	}))
}

func (fs *FavoritesSidebar) cancelManagement() bool {
	fs.mu.Lock()
	if fs.destroyed {
		fs.mu.Unlock()
		return false
	}
	active := fs.mode != favoritesSidebarModeNone || fs.confirmDelete
	fs.mode = favoritesSidebarModeNone
	fs.editingID = 0
	fs.confirmDelete = false
	if active {
		fs.notice = ""
	}
	fs.mu.Unlock()
	if fs.formBox != nil {
		fs.formBox.SetVisible(false)
		clearBoxChildren(fs.formBox)
	}
	fs.rebuildList()
	return active
}

func (fs *FavoritesSidebar) submitForm() bool {
	if fs == nil {
		return false
	}
	fs.mu.RLock()
	mode := fs.mode
	id := fs.editingID
	uc := fs.favoritesUC
	ctx := fs.ctx
	fs.mu.RUnlock()
	if uc == nil || (mode != favoritesSidebarModeAdd && mode != favoritesSidebarModeEdit) {
		return false
	}
	url, title, tagsText, shortcutText := fs.formValues()
	if mode == favoritesSidebarModeAdd {
		tags, err := parseTagIDs(tagsText)
		if err != nil {
			fs.setNotice(err.Error())
			return true
		}
		key, err := parseShortcut(shortcutText)
		if err != nil {
			fs.setNotice(err.Error())
			return true
		}
		input := dto.FavoriteCreateInput{URL: url, Title: title, Tags: tags}
		fav, err := uc.AddFavorite(ctx, input)
		if err != nil {
			fs.setNotice(err.Error())
			return true
		}
		if key != nil && fav != nil {
			if err := uc.SetShortcut(ctx, fav.ID, key); err != nil {
				fs.setNotice(err.Error())
				return true
			}
		}
	} else {
		key, err := parseShortcut(shortcutText)
		if err != nil {
			fs.setNotice(err.Error())
			return true
		}
		_, err = uc.UpdateFavorite(ctx, dto.FavoriteUpdateInput{ID: id, Title: title, ShortcutKey: key, ShortcutKeySet: shortcutText != ""})
		if err != nil {
			fs.setNotice(err.Error())
			return true
		}
	}
	fs.cancelManagement()
	fs.startLoad()
	return true
}

func (fs *FavoritesSidebar) enterTagMode() {
	fs.setModeNotice(favoritesSidebarModeTag, "Tag mode: press 1-9 to toggle visible tag for selected favorite")
}
func (fs *FavoritesSidebar) enterShortcutMode() {
	fs.setModeNotice(favoritesSidebarModeShortcut, "Shortcut mode: press 1-9 to assign, Backspace/Delete to clear")
}

func (fs *FavoritesSidebar) setModeNotice(mode favoritesSidebarMode, notice string) {
	fs.mu.Lock()
	if !fs.destroyed {
		fs.mode = mode
		fs.confirmDelete = false
		fs.notice = notice
	}
	fs.mu.Unlock()
	fs.rebuildList()
}

func (fs *FavoritesSidebar) toggleFavoriteTagByOrdinal(ord int) bool {
	fav := fs.selectedFavorite()
	if fav == nil || ord < 1 {
		fs.setNotice("Select a favorite and tag 1-9")
		return true
	}
	fs.mu.RLock()
	if ord > len(fs.allTags) || fs.favoritesUC == nil {
		fs.mu.RUnlock()
		fs.setNotice("No tag for key")
		return true
	}
	tagID := fs.allTags[ord-1].ID
	uc := fs.favoritesUC
	ctx := fs.ctx
	fs.mu.RUnlock()
	has := false
	for _, tag := range fav.Tags {
		if tag.ID == tagID {
			has = true
			break
		}
	}
	var err error
	if has {
		err = uc.UntagFavorite(ctx, fav.ID, tagID)
	} else {
		err = uc.TagFavorite(ctx, fav.ID, tagID)
	}
	if err != nil {
		fs.setNotice(err.Error())
		return true
	}
	fs.startLoad()
	return true
}

func (fs *FavoritesSidebar) setShortcutKey(key *int) bool {
	fav := fs.selectedFavorite()
	if fav == nil {
		fs.setNotice("Select a favorite for shortcut")
		return true
	}
	fs.mu.RLock()
	uc := fs.favoritesUC
	ctx := fs.ctx
	fs.mu.RUnlock()
	if uc == nil {
		return true
	}
	if err := uc.SetShortcut(ctx, fav.ID, key); err != nil {
		fs.setNotice(err.Error())
		return true
	}
	fs.startLoad()
	return true
}

func (fs *FavoritesSidebar) requestDeleteConfirmation() bool {
	if fs.selectedFavorite() == nil {
		fs.setNotice("Select a favorite to delete")
		return true
	}
	fs.mu.Lock()
	if !fs.destroyed {
		fs.confirmDelete = true
		fs.notice = "Delete selected favorite? Press Delete again to confirm, Esc to cancel."
	}
	fs.mu.Unlock()
	fs.rebuildList()
	return true
}

func (fs *FavoritesSidebar) formModeActive() bool {
	if fs == nil {
		return false
	}
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.mode == favoritesSidebarModeAdd || fs.mode == favoritesSidebarModeEdit
}

func (fs *FavoritesSidebar) confirmDeleteActive() bool {
	if fs == nil {
		return false
	}
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	return fs.confirmDelete
}

func (fs *FavoritesSidebar) confirmDeleteFavorite() bool {
	fav := fs.selectedFavorite()
	if fav == nil {
		return fs.requestDeleteConfirmation()
	}
	fs.mu.RLock()
	uc := fs.favoritesUC
	ctx := fs.ctx
	fs.mu.RUnlock()
	if uc == nil {
		return true
	}
	if err := uc.DeleteFavorite(ctx, fav.ID); err != nil {
		fs.setNotice(err.Error())
		return true
	}
	fs.cancelManagement()
	fs.startLoad()
	return true
}

func (fs *FavoritesSidebar) selectedFavorite() *entity.Favorite {
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	if len(fs.displayRows) == 1 {
		return fs.displayRows[0].Favorite
	}
	idx := firstSelectableIndex(fs.displayRows)
	if fs.listBox != nil {
		if row := fs.listBox.GetSelectedRow(); row != nil {
			idx = row.GetIndex()
		}
	}
	if idx >= 0 && idx < len(fs.displayRows) {
		return fs.displayRows[idx].Favorite
	}
	return nil
}

func (fs *FavoritesSidebar) setNotice(notice string) {
	fs.mu.Lock()
	if !fs.destroyed {
		fs.notice = notice
	}
	fs.mu.Unlock()
	fs.rebuildList()
}

func (fs *FavoritesSidebar) formValues() (string, string, string, string) {
	url, title, tagsText, shortcut := fs.formURL, fs.formTitle, fs.formTags, fs.formShortcut
	if fs.formURLEntry != nil {
		url = fs.formURLEntry.GetText()
	}
	if fs.formTitleEntry != nil {
		title = fs.formTitleEntry.GetText()
	}
	if fs.formTagsEntry != nil {
		tagsText = fs.formTagsEntry.GetText()
	}
	if fs.formShortcutEntry != nil {
		shortcut = fs.formShortcutEntry.GetText()
	}
	return strings.TrimSpace(url), strings.TrimSpace(title), strings.TrimSpace(tagsText), strings.TrimSpace(shortcut)
}

func parseTagIDs(text string) ([]entity.TagID, error) {
	if text == "" {
		return nil, nil
	}
	parts := strings.Split(text, ",")
	ids := make([]entity.TagID, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			return nil, fmt.Errorf("invalid tag ID %q", part)
		}
		v, err := strconv.Atoi(trimmed)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("invalid tag ID %q", trimmed)
		}
		ids = append(ids, entity.TagID(v))
	}
	return ids, nil
}

func parseShortcut(text string) (*int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	v, err := strconv.Atoi(text)
	if err != nil || v < 1 || v > 9 {
		return nil, fmt.Errorf("invalid shortcut %q (use 1-9)", text)
	}
	return &v, nil
}

func joinFavoriteTagIDs(tags []entity.Tag) string {
	parts := make([]string, 0, len(tags))
	for _, tag := range tags {
		parts = append(parts, fmt.Sprint(tag.ID))
	}
	return strings.Join(parts, ",")
}
