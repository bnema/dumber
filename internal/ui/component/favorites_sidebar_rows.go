package component

import (
	"fmt"

	"github.com/bnema/puregotk/v4/gtk"
	"github.com/bnema/puregotk/v4/pango"

	"github.com/bnema/dumber/internal/domain/entity"
)

type favoriteSidebarDisplayRowKind int

const (
	favoriteSidebarDisplayRowFavorite favoriteSidebarDisplayRowKind = iota
)

type favoriteSidebarDisplayRow struct {
	Kind       favoriteSidebarDisplayRowKind
	FavoriteID entity.FavoriteID
	TagID      entity.TagID
	URL        string
	Favorite   *entity.Favorite
	Selectable bool
}

func buildFavoriteSidebarDisplayRows(favorites []*entity.Favorite) []favoriteSidebarDisplayRow {
	rows := make([]favoriteSidebarDisplayRow, 0, len(favorites))
	for _, fav := range favorites {
		if fav == nil {
			continue
		}
		rows = append(rows, favoriteSidebarDisplayRow{
			Kind:       favoriteSidebarDisplayRowFavorite,
			FavoriteID: fav.ID,
			URL:        fav.URL,
			Favorite:   fav,
			Selectable: fav.URL != "",
		})
	}
	return rows
}

func firstSelectableIndex(rows []favoriteSidebarDisplayRow) int {
	for i := range rows {
		if rows[i].Selectable {
			return i
		}
	}
	return -1
}

func lastSelectableIndex(rows []favoriteSidebarDisplayRow) int {
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].Selectable {
			return i
		}
	}
	return -1
}

func nextSelectableIndex(rows []favoriteSidebarDisplayRow, current, direction int) int {
	if direction != -1 && direction != 1 {
		return -1
	}
	for i := current + direction; i >= 0 && i < len(rows); i += direction {
		if rows[i].Selectable {
			return i
		}
	}
	return -1
}

func (fs *FavoritesSidebar) renderTags() {
	if fs == nil || fs.tagBox == nil {
		return
	}
	fs.mu.RLock()
	if fs.destroyed {
		fs.mu.RUnlock()
		return
	}
	tags := append([]*entity.Tag(nil), fs.allTags...)
	selectedTagIDs := make(map[entity.TagID]struct{}, len(fs.selectedTagIDs))
	for id := range fs.selectedTagIDs {
		selectedTagIDs[id] = struct{}{}
	}
	tagBox := fs.tagBox
	fs.mu.RUnlock()

	clearBoxChildren(tagBox)
	callbacks := make([]interface{}, 0, len(tags)+1)

	allButton := gtk.NewButtonWithLabel("All")
	if allButton != nil {
		if len(selectedTagIDs) == 0 {
			allButton.AddCssClass("suggested-action")
		}
		cb := func(_ gtk.Button) {
			fs.mu.Lock()
			if fs.destroyed {
				fs.mu.Unlock()
				return
			}
			fs.selectedTagIDs = make(map[entity.TagID]struct{})
			fs.rebuildDisplayRowsLocked()
			fs.mu.Unlock()
			fs.renderTags()
			fs.rebuildList()
		}
		callbacks = append(callbacks, cb)
		allButton.ConnectClicked(&cb)
		tagBox.Append(&allButton.Widget)
	}

	for _, tag := range tags {
		if tag == nil {
			continue
		}
		t := tag
		button := gtk.NewButtonWithLabel(t.Name)
		if button == nil {
			continue
		}
		if _, ok := selectedTagIDs[t.ID]; ok {
			button.AddCssClass("suggested-action")
		}
		cb := func(_ gtk.Button) {
			fs.toggleTag(t.ID)
		}
		callbacks = append(callbacks, cb)
		button.ConnectClicked(&cb)
		tagBox.Append(&button.Widget)
	}

	fs.mu.Lock()
	if !fs.destroyed {
		fs.tagCallbacks = callbacks
	}
	fs.mu.Unlock()
}

func (fs *FavoritesSidebar) toggleTag(tagID entity.TagID) {
	if fs == nil {
		return
	}
	fs.mu.Lock()
	if fs.destroyed {
		fs.mu.Unlock()
		return
	}
	if fs.selectedTagIDs == nil {
		fs.selectedTagIDs = make(map[entity.TagID]struct{})
	}
	if _, ok := fs.selectedTagIDs[tagID]; ok {
		delete(fs.selectedTagIDs, tagID)
	} else {
		fs.selectedTagIDs[tagID] = struct{}{}
	}
	fs.rebuildDisplayRowsLocked()
	fs.mu.Unlock()
	fs.renderTags()
	fs.rebuildList()
}

func (fs *FavoritesSidebar) rebuildList() {
	if fs == nil {
		return
	}
	fs.mu.RLock()
	if fs.destroyed || fs.listBox == nil {
		fs.mu.RUnlock()
		return
	}
	listBox := fs.listBox
	rows := append([]favoriteSidebarDisplayRow(nil), fs.displayRows...)
	notice := fs.notice
	query := fs.currentQuery
	fs.mu.RUnlock()

	listBox.RemoveAll()
	if len(rows) == 0 {
		text := "No favorites"
		if notice != "" {
			text = notice
		} else if query != "" {
			text = noResultsText(query)
		}
		fs.appendNoticeRow(listBox, text)
		return
	}
	for _, row := range rows {
		fs.appendFavoriteRow(listBox, row.Favorite)
	}
	fs.ensureAtLeastOneSelectionInListBox(listBox)
}

func (fs *FavoritesSidebar) appendNoticeRow(listBox *gtk.ListBox, text string) {
	label := gtk.NewLabel(&text)
	if label == nil {
		return
	}
	label.AddCssClass("favorites-sidebar-empty")
	label.SetXalign(0.0)
	row := gtk.NewListBoxRow()
	if row == nil {
		return
	}
	row.SetSelectable(false)
	row.SetCanFocus(false)
	row.SetActivatable(false)
	row.SetChild(&label.Widget)
	listBox.Append(&row.Widget)
}

func (fs *FavoritesSidebar) appendFavoriteRow(listBox *gtk.ListBox, fav *entity.Favorite) {
	if fav == nil {
		return
	}
	rowBox := gtk.NewBox(gtk.OrientationVerticalValue, 2)
	if rowBox == nil {
		return
	}
	rowBox.SetHexpand(true)

	title := gtk.NewLabel(nil)
	if title == nil {
		return
	}
	title.AddCssClass("favorites-sidebar-row-title")
	title.SetText(safeSidebarString(fav.Title, fav.URL))
	title.SetXalign(0.0)
	title.SetHexpand(true)
	title.SetEllipsize(pango.EllipsizeEndValue)
	rowBox.Append(&title.Widget)

	sub := gtk.NewBox(gtk.OrientationHorizontalValue, 4)
	if sub == nil {
		return
	}
	sub.SetHexpand(true)
	url := gtk.NewLabel(nil)
	if url == nil {
		return
	}
	url.AddCssClass("favorites-sidebar-row-subtitle")
	url.SetText(readableURL(fav.URL))
	url.SetXalign(0.0)
	url.SetHexpand(true)
	url.SetEllipsize(pango.EllipsizeEndValue)
	sub.Append(&url.Widget)
	if fav.ShortcutKey != nil {
		badge := fmt.Sprintf("Shortcut %d", *fav.ShortcutKey)
		badgeLabel := gtk.NewLabel(&badge)
		if badgeLabel != nil {
			badgeLabel.AddCssClass("favorites-sidebar-shortcut-badge")
			sub.Append(&badgeLabel.Widget)
		}
	}
	rowBox.Append(&sub.Widget)

	if len(fav.Tags) > 0 {
		tags := gtk.NewBox(gtk.OrientationHorizontalValue, 3)
		if tags != nil {
			for _, tag := range fav.Tags {
				name := tag.Name
				label := gtk.NewLabel(&name)
				if label != nil {
					label.AddCssClass("favorites-sidebar-tag-chip")
					tags.Append(&label.Widget)
				}
			}
			rowBox.Append(&tags.Widget)
		}
	}

	row := gtk.NewListBoxRow()
	if row == nil {
		return
	}
	row.AddCssClass("favorites-sidebar-row")
	row.SetSelectable(true)
	row.SetActivatable(true)
	row.SetCanFocus(true)
	row.SetFocusOnClick(true)
	row.SetChild(&rowBox.Widget)
	listBox.Append(&row.Widget)
}

func (fs *FavoritesSidebar) ensureAtLeastOneSelectionInListBox(listBox *gtk.ListBox) {
	if listBox == nil || listBox.GetSelectedRow() != nil {
		return
	}
	fs.mu.RLock()
	index := firstSelectableIndex(fs.displayRows)
	fs.mu.RUnlock()
	if row := listBox.GetRowAtIndex(index); row != nil {
		listBox.SelectRow(row)
	}
}

func (fs *FavoritesSidebar) rowURLAt(index int) string {
	if index < 0 || index >= len(fs.displayRows) || !fs.displayRows[index].Selectable {
		return ""
	}
	return fs.displayRows[index].URL
}
