package systemviews

import (
	"fmt"
	"html"
	"strings"

	"github.com/bnema/dumber/internal/domain/entity"
)

type favoritesRenderData struct {
	Favorites    []*entity.Favorite
	Folders      []*entity.Folder
	Tags         []*entity.Tag
	FolderFilter *entity.FolderID
	TagFilter    *entity.TagID
	Notice       string
	Error        string
}

func favoritesHTML(data favoritesRenderData) string {
	favoritesSummary := fmt.Sprintf("%d favorites · %d folders · %d tags", len(data.Favorites), len(data.Folders), len(data.Tags))
	foldersSummary := fmt.Sprintf("%d folders", len(data.Folders))
	tagsSummary := fmt.Sprintf("%d tags", len(data.Tags))
	sections := []string{
		favoritesStatusHTML(data),
		sectionHTML("sv-favorites-controls", "Manage favorites", favoritesControlsHTML(data)),
		sectionHTML("sv-favorites-list", "Favorites", metaHTML(favoritesSummary)+listHTML(favoriteItemsHTML(data.Favorites, data.Folders, data.Tags), "No favorites")),
		sectionHTML("sv-folders-list", "Folders", metaHTML(foldersSummary)+folderCreateHTML()+listHTML(folderItemsHTML(data.Folders), "No folders")),
		sectionHTML("sv-tags-list", "Tags", metaHTML(tagsSummary)+tagCreateHTML()+listHTML(tagItemsHTML(data.Tags), "No tags")),
	}
	return strings.Join(sections, "")
}

func favoritesStatusHTML(data favoritesRenderData) string {
	var out strings.Builder
	if strings.TrimSpace(data.Notice) != "" {
		out.WriteString(fmt.Sprintf(`<div class="sv-alert sv-alert-success" role="status">%s</div>`, html.EscapeString(data.Notice)))
	}
	if strings.TrimSpace(data.Error) != "" {
		out.WriteString(fmt.Sprintf(`<div class="sv-alert sv-alert-error" role="alert">%s</div>`, html.EscapeString(data.Error)))
	}
	return out.String()
}

func favoritesControlsHTML(data favoritesRenderData) string {
	var out strings.Builder
	out.WriteString(`<form class="sv-card-form" data-sv-action="favorite.create">`)
	out.WriteString(`<label><span>URL</span><input name="url" type="url" placeholder="https://example.com" required></label>`)
	out.WriteString(`<label><span>Title</span><input name="title" type="text" placeholder="Optional title"></label>`)
	out.WriteString(folderSelectHTML("folder_id", data.Folders, nil, "Folder"))
	out.WriteString(`<button class="sv-button" type="submit">Add favorite</button>`)
	out.WriteString(`</form>`)

	out.WriteString(`<div class="sv-filter-panel">`)
	out.WriteString(`<div class="sv-button-row"><span class="sv-meta">Folders</span>`)
	out.WriteString(favoriteFilterButton("All", "favorite.clearFilters", nil, "", data.FolderFilter == nil && data.TagFilter == nil))
	out.WriteString(favoriteFilterButton("Unfiled", "favorite.filterFolder", map[string]string{"folder_id": "root"}, "", data.FolderFilter != nil && *data.FolderFilter == 0))
	for _, folder := range data.Folders {
		if folder == nil {
			continue
		}
		active := data.FolderFilter != nil && *data.FolderFilter == folder.ID
		out.WriteString(favoriteFilterButton(folderDisplayName(folder), "favorite.filterFolder", map[string]string{"folder_id": fmt.Sprintf("%d", folder.ID)}, "", active))
	}
	out.WriteString(`</div>`)

	out.WriteString(`<div class="sv-button-row"><span class="sv-meta">Tags</span>`)
	for _, tag := range data.Tags {
		if tag == nil {
			continue
		}
		active := data.TagFilter != nil && *data.TagFilter == tag.ID
		out.WriteString(favoriteFilterButton(tag.Name, "favorite.filterTag", map[string]string{"tag_id": fmt.Sprintf("%d", tag.ID)}, tag.Color, active))
	}
	out.WriteString(`</div></div>`)
	out.WriteString(`<p class="sv-meta sv-key-hints">Keys: <kbd>j</kbd>/<kbd>k</kbd> move, <kbd>Enter</kbd> open, <kbd>e</kbd> edit title, <kbd>d</kbd> delete focused favorite.</p>`)
	return out.String()
}

func favoriteFilterButton(label, action string, data map[string]string, color string, active bool) string {
	classes := "sv-button sv-button-secondary"
	if active {
		classes += " sv-button-active"
	}
	attrs := []string{`type="button"`, fmt.Sprintf(`class="%s"`, html.EscapeString(classes)), fmt.Sprintf(`data-sv-action="%s"`, html.EscapeString(action))}
	for key, value := range data {
		attrs = append(attrs, fmt.Sprintf(`data-%s="%s"`, html.EscapeString(kebabDataKey(key)), html.EscapeString(value)))
	}
	style := ""
	if strings.TrimSpace(color) != "" {
		style = fmt.Sprintf(`<span class="sv-tag-dot" style="background:%s"></span>`, html.EscapeString(color))
	}
	return fmt.Sprintf(`<button %s>%s%s</button>`, strings.Join(attrs, " "), style, html.EscapeString(label))
}

func favoriteItemsHTML(favorites []*entity.Favorite, folders []*entity.Folder, tags []*entity.Tag) string {
	var items strings.Builder
	for _, favorite := range favorites {
		if favorite == nil {
			continue
		}
		items.WriteString(listRowHTML(favoriteItemHTML(favorite, folders, tags)))
	}
	return items.String()
}

func favoriteItemHTML(favorite *entity.Favorite, folders []*entity.Folder, tags []*entity.Tag) string {
	label := strings.TrimSpace(favorite.Title)
	if label == "" {
		label = favorite.URL
	}

	return fmt.Sprintf(`<article class="sv-favorite-item" data-sv-favorite-row data-favorite-id="%d"><div class="sv-favorite-header"><div><a class="sv-link sv-favorite-open" href="%s">%s</a><p class="sv-history-url">%s</p>%s</div><div class="sv-history-row-actions"><a class="sv-button sv-button-secondary" href="%s">Open</a><button type="button" class="sv-button sv-button-secondary sv-button-danger" data-sv-action="favorite.delete" data-id="%d" data-sv-confirm="Delete this favorite?">Delete</button></div></div>%s%s</article>`,
		favorite.ID,
		html.EscapeString(sanitizeHref(favorite.URL)),
		html.EscapeString(label),
		html.EscapeString(favorite.URL),
		favoriteMetaHTML(favorite, folders),
		html.EscapeString(sanitizeHref(favorite.URL)),
		favorite.ID,
		favoriteEditHTML(favorite, folders),
		favoriteTagControlsHTML(favorite, tags),
	)
}

func favoriteMetaHTML(favorite *entity.Favorite, folders []*entity.Folder) string {
	parts := []string{}
	if favorite.ShortcutKey != nil {
		parts = append(parts, fmt.Sprintf("Shortcut %d", *favorite.ShortcutKey))
	}
	if name := folderNameForID(folders, favorite.FolderID); name != "" {
		parts = append(parts, "Folder "+name)
	}
	if len(parts) == 0 {
		return ""
	}
	return `<p class="sv-meta">` + html.EscapeString(strings.Join(parts, " · ")) + `</p>`
}

func favoriteEditHTML(favorite *entity.Favorite, folders []*entity.Folder) string {
	return fmt.Sprintf(`<form class="sv-inline-form" data-sv-action="favorite.update"><input type="hidden" name="id" value="%d"><label><span>Title</span><input name="title" type="text" value="%s"></label><label><span>Favicon</span><input name="favicon_url" type="url" value="%s"></label>%s%s<button class="sv-button sv-button-secondary" type="submit">Save</button></form>`,
		favorite.ID,
		html.EscapeString(favorite.Title),
		html.EscapeString(favorite.FaviconURL),
		folderSelectHTML("folder_id", folders, favorite.FolderID, "Folder"),
		shortcutSelectHTML(favorite.ShortcutKey),
	)
}

func favoriteTagControlsHTML(favorite *entity.Favorite, tags []*entity.Tag) string {
	if len(tags) == 0 {
		return ""
	}
	var out strings.Builder
	out.WriteString(`<div class="sv-tag-controls"><span class="sv-meta">Tags</span>`)
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		action := "tag.assign"
		label := "+ " + tag.Name
		classes := "sv-button sv-button-secondary"
		if favorite.HasTag(tag.ID) {
			action = "tag.remove"
			label = "✓ " + tag.Name
			classes += " sv-button-active"
		}
		out.WriteString(fmt.Sprintf(`<button type="button" class="%s" data-sv-action="%s" data-favorite-id="%d" data-tag-id="%d"><span class="sv-tag-dot" style="background:%s"></span>%s</button>`,
			html.EscapeString(classes),
			html.EscapeString(action),
			favorite.ID,
			tag.ID,
			html.EscapeString(tag.Color),
			html.EscapeString(label),
		))
	}
	out.WriteString(`</div>`)
	return out.String()
}

func folderCreateHTML() string {
	return `<form class="sv-inline-form" data-sv-action="folder.create"><label><span>Name</span><input name="name" type="text" required></label><label><span>Icon</span><input name="icon" type="text" placeholder="Optional"></label><button class="sv-button sv-button-secondary" type="submit">Create folder</button></form>`
}

func folderItemsHTML(folders []*entity.Folder) string {
	var items strings.Builder
	for _, folder := range folders {
		if folder == nil {
			continue
		}
		items.WriteString(listRowHTML(folderItemHTML(folder)))
	}
	return items.String()
}

func folderItemHTML(folder *entity.Folder) string {
	return fmt.Sprintf(`<form class="sv-inline-form" data-sv-action="folder.update"><input type="hidden" name="id" value="%d"><label><span>Name</span><input name="name" type="text" value="%s" required></label><label><span>Icon</span><input name="icon" type="text" value="%s"></label><button class="sv-button sv-button-secondary" type="submit">Save</button><button type="button" class="sv-button sv-button-secondary sv-button-danger" data-sv-action="folder.delete" data-id="%d" data-sv-confirm="Delete this folder? Favorites will move out of it.">Delete</button></form>`,
		folder.ID,
		html.EscapeString(folder.Name),
		html.EscapeString(folder.Icon),
		folder.ID,
	)
}

func tagCreateHTML() string {
	return `<form class="sv-inline-form" data-sv-action="tag.create"><label><span>Name</span><input name="name" type="text" required></label><label><span>Color</span><input name="color" type="text" placeholder="#808080"></label><button class="sv-button sv-button-secondary" type="submit">Create tag</button></form>`
}

func tagItemsHTML(tags []*entity.Tag) string {
	var items strings.Builder
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		items.WriteString(listRowHTML(tagItemHTML(tag)))
	}
	return items.String()
}

func tagItemHTML(tag *entity.Tag) string {
	return fmt.Sprintf(`<form class="sv-inline-form" data-sv-action="tag.update"><input type="hidden" name="id" value="%d"><span class="sv-tag-dot" style="background:%s"></span><label><span>Name</span><input name="name" type="text" value="%s" required></label><label><span>Color</span><input name="color" type="text" value="%s"></label><button class="sv-button sv-button-secondary" type="submit">Save</button><button type="button" class="sv-button sv-button-secondary sv-button-danger" data-sv-action="tag.delete" data-id="%d" data-sv-confirm="Delete this tag?">Delete</button></form>`,
		tag.ID,
		html.EscapeString(tag.Color),
		html.EscapeString(tag.Name),
		html.EscapeString(tag.Color),
		tag.ID,
	)
}

func folderSelectHTML(name string, folders []*entity.Folder, selected *entity.FolderID, label string) string {
	var out strings.Builder
	out.WriteString(fmt.Sprintf(`<label><span>%s</span><select name="%s"><option value="">Unfiled</option>`, html.EscapeString(label), html.EscapeString(name)))
	for _, folder := range folders {
		if folder == nil {
			continue
		}
		selectedAttr := ""
		if selected != nil && *selected == folder.ID {
			selectedAttr = ` selected`
		}
		out.WriteString(fmt.Sprintf(`<option value="%d"%s>%s</option>`, folder.ID, selectedAttr, html.EscapeString(folderDisplayName(folder))))
	}
	out.WriteString(`</select></label>`)
	return out.String()
}

func shortcutSelectHTML(selected *int) string {
	var out strings.Builder
	out.WriteString(`<label><span>Shortcut</span><select name="shortcut_key"><option value="">None</option>`)
	for i := 1; i <= 9; i++ {
		selectedAttr := ""
		if selected != nil && *selected == i {
			selectedAttr = ` selected`
		}
		out.WriteString(fmt.Sprintf(`<option value="%d"%s>%d</option>`, i, selectedAttr, i))
	}
	out.WriteString(`</select></label>`)
	return out.String()
}

func folderDisplayName(folder *entity.Folder) string {
	if folder == nil {
		return ""
	}
	if strings.TrimSpace(folder.Icon) != "" {
		return folder.Icon + " " + folder.Name
	}
	return folder.Name
}

func folderNameForID(folders []*entity.Folder, id *entity.FolderID) string {
	if id == nil {
		return "Unfiled"
	}
	for _, folder := range folders {
		if folder != nil && folder.ID == *id {
			return folderDisplayName(folder)
		}
	}
	return "Unknown"
}

func filterFavorites(favorites []*entity.Favorite, folderID *entity.FolderID, tagID *entity.TagID) []*entity.Favorite {
	if folderID == nil && tagID == nil {
		return favorites
	}
	out := make([]*entity.Favorite, 0, len(favorites))
	for _, favorite := range favorites {
		if favorite == nil {
			continue
		}
		if folderID != nil {
			if *folderID == 0 {
				if favorite.FolderID != nil {
					continue
				}
			} else if favorite.FolderID == nil || *favorite.FolderID != *folderID {
				continue
			}
		}
		if tagID != nil && !favorite.HasTag(*tagID) {
			continue
		}
		out = append(out, favorite)
	}
	return out
}
