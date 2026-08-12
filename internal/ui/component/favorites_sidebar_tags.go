package component

import (
	"strings"

	"github.com/bnema/puregotk/v4/gtk"

	"github.com/bnema/dumber/internal/domain/entity"
)

func (fs *FavoritesSidebar) showCreateTagPrompt() {
	if fs == nil || fs.tagPromptBox == nil {
		return
	}
	fs.clearTagPromptContent()
	entry := gtk.NewEntry()
	if entry == nil {
		return
	}
	placeholder := "New tag name"
	entry.SetPlaceholderText(&placeholder)
	entry.AddCssClass("favorites-sidebar-tag-name")
	fs.trackTextInputFocus(&entry.Widget)
	entry.SetHexpand(true)
	callbacks := make([]any, 0, 2)
	activateCb := func(gtk.Entry) { fs.submitTagName(entry.GetText()) }
	callbacks = append(callbacks, activateCb)
	entry.ConnectActivate(&activateCb)

	save := gtk.NewButtonWithLabel("Create")
	if save == nil {
		return
	}
	save.AddCssClass("favorites-sidebar-tag-create")
	cb := func(_ gtk.Button) { fs.createTag(entry.GetText()) }
	callbacks = append(callbacks, cb)
	save.ConnectClicked(&cb)

	fs.tagNameEntry = entry
	fs.tagPromptSaveBtn = save
	fs.storeTagPromptCallbacks(callbacks)
	fs.tagPromptBox.Append(&entry.Widget)
	fs.tagPromptBox.Append(&save.Widget)
	fs.tagPromptBox.SetVisible(true)
	fs.mu.Lock()
	if !fs.destroyed {
		fs.mode = favoritesSidebarModeCreateTag
	}
	fs.mu.Unlock()
	entry.GrabFocus()
}

func (fs *FavoritesSidebar) submitTagName(name string) bool {
	return fs.createTag(name)
}

func (fs *FavoritesSidebar) createTag(name string) bool {
	if fs == nil {
		return false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		fs.setNotice("Tag name is required")
		return true
	}
	fs.mu.RLock()
	uc, ctx := fs.favoritesUC, fs.ctx
	fs.mu.RUnlock()
	if uc == nil {
		return false
	}
	if _, err := uc.AddTag(ctx, name, ""); err != nil {
		fs.setNotice(err.Error())
		return true
	}
	fs.hideTagPrompt()
	fs.startLoad()
	return true
}

func (fs *FavoritesSidebar) showTagBindingPicker() bool {
	if fs == nil || fs.tagPromptBox == nil {
		return false
	}
	fav := fs.selectedFavorite()
	if fav == nil {
		fs.setNotice("Select a favorite to manage tags")
		return true
	}
	fs.mu.RLock()
	tags := append([]*entity.Tag(nil), fs.allTags...)
	fs.mu.RUnlock()
	fs.clearTagPromptContent()
	callbacks := make([]any, 0, len(tags))
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		t := *tag
		label := t.Name
		if favoriteHasTag(fav, t.ID) {
			label = "✓ " + label
		}
		button := gtk.NewButtonWithLabel(label)
		if button == nil {
			continue
		}
		button.AddCssClass("favorites-sidebar-tag-binding")
		cb := func(_ gtk.Button) { fs.toggleFavoriteTag(fav.ID, t.ID) }
		callbacks = append(callbacks, cb)
		button.ConnectClicked(&cb)
		fs.tagPromptBox.Append(&button.Widget)
	}
	fs.tagNameEntry = nil
	fs.tagPromptSaveBtn = nil
	fs.storeTagPromptCallbacks(callbacks)
	fs.tagPromptBox.SetVisible(true)
	fs.mu.Lock()
	if !fs.destroyed {
		fs.mode = favoritesSidebarModeBindTag
	}
	fs.mu.Unlock()
	return true
}

func (fs *FavoritesSidebar) hideTagPrompt() {
	if fs == nil || fs.tagPromptBox == nil {
		return
	}
	fs.clearTagPromptContent()
	fs.tagPromptBox.SetVisible(false)
	fs.tagNameEntry = nil
	fs.tagPromptSaveBtn = nil
	fs.mu.Lock()
	if !fs.destroyed && (fs.mode == favoritesSidebarModeCreateTag || fs.mode == favoritesSidebarModeBindTag) {
		fs.mode = favoritesSidebarModeNone
	}
	fs.mu.Unlock()
}

func (fs *FavoritesSidebar) clearTagPromptContent() {
	if fs == nil || fs.tagPromptBox == nil {
		return
	}
	clearBoxChildren(fs.tagPromptBox)
	fs.mu.Lock()
	fs.tagPromptCallbacks = nil
	fs.mu.Unlock()
}

func (fs *FavoritesSidebar) storeTagPromptCallbacks(callbacks []any) {
	if fs == nil {
		return
	}
	fs.mu.Lock()
	if !fs.destroyed {
		fs.tagPromptCallbacks = callbacks
	}
	fs.mu.Unlock()
}

func (fs *FavoritesSidebar) toggleFavoriteTag(favoriteID entity.FavoriteID, tagID entity.TagID) {
	fs.mu.RLock()
	uc, ctx := fs.favoritesUC, fs.ctx
	var has bool
	for _, fav := range fs.allFavorites {
		if fav == nil || fav.ID != favoriteID {
			continue
		}
		has = favoriteHasTag(fav, tagID)
		break
	}
	fs.mu.RUnlock()
	if uc == nil {
		return
	}
	var err error
	if has {
		err = uc.UntagFavorite(ctx, favoriteID, tagID)
	} else {
		err = uc.TagFavorite(ctx, favoriteID, tagID)
	}
	if err != nil {
		fs.setNotice(err.Error())
		return
	}
	fs.hideTagPrompt()
	fs.startLoad()
}

func favoriteHasTag(fav *entity.Favorite, tagID entity.TagID) bool {
	if fav == nil {
		return false
	}
	for _, tag := range fav.Tags {
		if tag.ID == tagID {
			return true
		}
	}
	return false
}

func (fs *FavoritesSidebar) focusTagControl(index int) {
	if fs == nil || index < 0 || index >= len(fs.tagControls) || fs.tagControls[index] == nil {
		return
	}
	fs.mu.Lock()
	if !fs.destroyed {
		fs.focusZone = favoritesSidebarFocusTags
	}
	fs.mu.Unlock()
	fs.tagControls[index].GrabFocus()
}

func (fs *FavoritesSidebar) focusedTagControlIndex() int {
	if fs == nil {
		return -1
	}
	for index, control := range fs.tagControls {
		if control != nil && control.HasFocus() {
			return index
		}
	}
	return -1
}

func (fs *FavoritesSidebar) createTagFromFocusedEntry() bool {
	if fs == nil || fs.tagNameEntry == nil || !fs.tagNameEntry.HasFocus() {
		return false
	}
	return fs.createTag(fs.tagNameEntry.GetText())
}
