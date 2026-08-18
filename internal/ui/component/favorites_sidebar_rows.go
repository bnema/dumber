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
	tagBox, tags, selectedTagIDs, ok := fs.tagRenderState()
	if !ok {
		return
	}
	clearBoxChildren(tagBox)
	callbacks := make([]any, 0, len(tags)+2)
	controls := make([]*gtk.Button, 0, len(tags)+2)

	fs.appendTagFilter(tagBox, "All", len(selectedTagIDs) == 0, fs.clearTagFilters, &callbacks, &controls)
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		t := tag
		_, selected := selectedTagIDs[t.ID]
		fs.appendTagFilter(tagBox, t.Name, selected, func() { fs.toggleTag(t.ID) }, &callbacks, &controls)
	}
	fs.appendTagFilter(tagBox, "+", false, fs.showCreateTagPrompt, &callbacks, &controls)
	if len(controls) > 0 {
		controls[len(controls)-1].AddCssClass("favorites-sidebar-tag-add")
	}
	fs.storeTagControls(callbacks, controls)
}

func (fs *FavoritesSidebar) tagRenderState() (*gtk.Box, []*entity.Tag, map[entity.TagID]struct{}, bool) {
	if fs == nil || fs.tagBox == nil {
		return nil, nil, nil, false
	}
	fs.mu.RLock()
	defer fs.mu.RUnlock()
	if fs.destroyed {
		return nil, nil, nil, false
	}
	selected := make(map[entity.TagID]struct{}, len(fs.selectedTagIDs))
	for id := range fs.selectedTagIDs {
		selected[id] = struct{}{}
	}
	return fs.tagBox, append([]*entity.Tag(nil), fs.allTags...), selected, true
}

func (fs *FavoritesSidebar) appendTagFilter(
	box *gtk.Box,
	label string,
	selected bool,
	action func(),
	callbacks *[]any,
	controls *[]*gtk.Button,
) {
	button := gtk.NewButtonWithLabel(label)
	if button == nil {
		return
	}
	button.AddCssClass("favorites-sidebar-tag-filter")
	if selected {
		button.AddCssClass("favorites-sidebar-tag-filter-active")
	}
	callback := func(_ gtk.Button) { action() }
	*callbacks = append(*callbacks, callback)
	button.ConnectClicked(&callback)
	box.Append(&button.Widget)
	*controls = append(*controls, button)
}

func (fs *FavoritesSidebar) clearTagFilters() {
	if fs == nil {
		return
	}
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

func (fs *FavoritesSidebar) storeTagControls(callbacks []any, controls []*gtk.Button) {
	fs.mu.Lock()
	if !fs.destroyed {
		fs.tagCallbacks = callbacks
		fs.tagControls = controls
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
		for _, text := range favoriteSidebarNoticeRows(notice, query, false) {
			fs.appendNoticeRow(listBox, text)
		}
		return
	}
	for _, row := range rows {
		fs.appendFavoriteRow(listBox, row)
	}
	for _, text := range favoriteSidebarNoticeRows(notice, query, true) {
		fs.appendNoticeRow(listBox, text)
	}
	fs.ensureAtLeastOneSelectionInListBox(listBox)
}

func favoriteSidebarNoticeRows(notice, query string, hasRows bool) []string {
	if notice != "" {
		return []string{notice}
	}
	if hasRows {
		return nil
	}
	if query != "" {
		return []string{noResultsText(query)}
	}
	return []string{"No favorites"}
}

func (fs *FavoritesSidebar) appendNoticeRow(listBox *gtk.ListBox, text string) {
	label := gtk.NewLabel(&text)
	if label == nil {
		return
	}
	label.AddCssClass("sidebar-empty")
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

func (fs *FavoritesSidebar) appendFavoriteRow(listBox *gtk.ListBox, displayRow favoriteSidebarDisplayRow) {
	fav := displayRow.Favorite
	if fav == nil {
		return
	}
	rowBox := gtk.NewBox(gtk.OrientationVerticalValue, 2)
	if rowBox == nil {
		return
	}
	rowBox.SetHexpand(true)
	appendFavoriteRowTitle(rowBox, fav)
	appendFavoriteRowSubtitle(rowBox, fav)
	appendFavoriteRowTags(rowBox, fav.Tags)

	row := gtk.NewListBoxRow()
	if row == nil {
		return
	}
	row.AddCssClass("sidebar-row")
	row.AddCssClass("favorites-sidebar-row")
	row.SetSelectable(displayRow.Selectable)
	row.SetActivatable(displayRow.Selectable)
	row.SetCanFocus(displayRow.Selectable)
	row.SetFocusOnClick(displayRow.Selectable)
	row.SetChild(&rowBox.Widget)
	listBox.Append(&row.Widget)
}

func appendFavoriteRowTitle(rowBox *gtk.Box, fav *entity.Favorite) {
	title := gtk.NewLabel(nil)
	if title == nil {
		return
	}
	title.AddCssClass("sidebar-row-title")
	title.AddCssClass("favorites-sidebar-row-title")
	title.SetText(safeSidebarString(fav.Title, fav.URL))
	title.SetXalign(0.0)
	title.SetHexpand(true)
	title.SetEllipsize(pango.EllipsizeEndValue)
	rowBox.Append(&title.Widget)
}

func appendFavoriteRowSubtitle(rowBox *gtk.Box, fav *entity.Favorite) {
	sub := gtk.NewBox(gtk.OrientationHorizontalValue, 4)
	if sub == nil {
		return
	}
	sub.SetHexpand(true)
	url := gtk.NewLabel(nil)
	if url != nil {
		url.AddCssClass("sidebar-row-subtitle")
		url.AddCssClass("favorites-sidebar-row-subtitle")
		url.SetText(readableURL(fav.URL))
		url.SetXalign(0.0)
		url.SetHexpand(true)
		url.SetEllipsize(pango.EllipsizeEndValue)
		sub.Append(&url.Widget)
	}
	if fav.ShortcutKey != nil {
		badge := fmt.Sprintf("Shortcut %d", *fav.ShortcutKey)
		badgeLabel := gtk.NewLabel(&badge)
		if badgeLabel != nil {
			badgeLabel.AddCssClass("favorites-sidebar-shortcut-badge")
			sub.Append(&badgeLabel.Widget)
		}
	}
	rowBox.Append(&sub.Widget)
}

func appendFavoriteRowTags(rowBox *gtk.Box, favoriteTags []entity.Tag) {
	if len(favoriteTags) == 0 {
		return
	}
	tags := gtk.NewBox(gtk.OrientationHorizontalValue, 3)
	if tags == nil {
		return
	}
	tags.AddCssClass("favorites-sidebar-row-tags")
	for _, tag := range favoriteTags {
		name := tag.Name
		label := gtk.NewLabel(&name)
		if label != nil {
			label.AddCssClass("favorites-sidebar-tag-chip")
			tags.Append(&label.Widget)
		}
	}
	rowBox.Append(&tags.Widget)
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
