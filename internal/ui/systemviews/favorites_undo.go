package systemviews

import (
	"context"
	"slices"
	"strconv"
	"strings"
)

// Deferred deletion of favorites and tags: the item leaves the view at once
// and the service delete runs only when the undo toast is dismissed.
const (
	favoriteActionCommitDelete = "favorite.commitDelete"
	favoriteActionUndoDelete   = "favorite.undoDelete"
	tagActionCommitDelete      = "tag.commitDelete"
	tagActionUndoDelete        = "tag.undoDelete"

	favoritesUndoTimeout = historyUndoTimeout
)

func favoritesUndoAction(action DOMAction) bool {
	switch action.Action {
	case favoriteActionCommitDelete, favoriteActionUndoDelete, tagActionCommitDelete, tagActionUndoDelete:
		return true
	case favoriteActionDelete, tagActionDelete:
		return action.Data[undoDataKey] == "true"
	default:
		return false
	}
}

func (a *App) handleFavoritesUndoAction(ctx context.Context, event DOMAction) error {
	switch event.Action {
	case favoriteActionDelete:
		return a.deferFavoritesDelete(ctx, event, false)
	case tagActionDelete:
		return a.deferFavoritesDelete(ctx, event, true)
	case favoriteActionUndoDelete:
		return a.undoFavoritesDelete(ctx, event, false)
	case tagActionUndoDelete:
		return a.undoFavoritesDelete(ctx, event, true)
	case favoriteActionCommitDelete, tagActionCommitDelete:
		tag := event.Action == tagActionCommitDelete
		id, err := parsePositiveInt64(event.Data["id"], favoritesIDLabel(tag))
		if err != nil {
			return err
		}
		if tag {
			return a.commitFavoritesDeletes(ctx, nil, []int64{id})
		}
		return a.commitFavoritesDeletes(ctx, []int64{id}, nil)
	default:
		return nil
	}
}

func favoritesIDLabel(tag bool) string {
	if tag {
		return "tag id"
	}
	return "favorite id"
}

// deferFavoritesDelete hides a favorite (or a tag) and offers an undo
// toast. Without a toast UI it deletes immediately.
func (a *App) deferFavoritesDelete(ctx context.Context, event DOMAction, tag bool) error {
	id, err := parsePositiveInt64(event.Data["id"], favoritesIDLabel(tag))
	if err != nil {
		return err
	}
	toaster, canToast := a.deps.DOM.(DOMToaster)

	a.lockState()
	if a.closed {
		a.unlockState()
		return nil
	}
	if !canToast {
		defer a.unlockState()
		plain := DOMAction{Action: event.Action, Data: map[string]string{"id": event.Data["id"]}}
		return a.applyFavoritesActionLocked(ctx, plain)
	}
	pending := a.pendingFavoriteDeletes
	if tag {
		pending = a.pendingTagDeletes
	}
	if _, ok := pending[id]; ok {
		a.unlockState()
		return nil
	}
	var message string
	if tag {
		if a.pendingTagDeletes == nil {
			a.pendingTagDeletes = map[int64]string{}
		}
		name := a.tagNameLocked(id)
		a.pendingTagDeletes[id] = name
		if a.favoriteTagFilter != nil && int64(*a.favoriteTagFilter) == id {
			a.favoriteTagFilter = nil
		}
		message = "Deleted tag " + quotedLabel(name)
	} else {
		if a.pendingFavoriteDeletes == nil {
			a.pendingFavoriteDeletes = map[int64]string{}
		}
		name := a.favoriteLabelLocked(id)
		a.pendingFavoriteDeletes[id] = name
		message = "Deleted " + quotedLabel(name)
	}
	a.favoritesNotice = ""
	a.favoritesError = ""
	err = a.refreshFavoritesRegionsLocked(ctx, false)
	a.unlockState()
	if err != nil {
		return err
	}

	undo, commit := favoriteActionUndoDelete, favoriteActionCommitDelete
	if tag {
		undo, commit = tagActionUndoDelete, tagActionCommitDelete
	}
	idValue := strconv.FormatInt(id, 10)
	shown := toaster.ShowToast(ToastRequest{
		Message:     message,
		ActionLabel: "Undo",
		Timeout:     favoritesUndoTimeout,
		OnAction:    DOMAction{Action: undo, Data: map[string]string{"id": idValue}},
		OnDismiss:   DOMAction{Action: commit, Data: map[string]string{"id": idValue}},
	})
	if shown {
		return nil
	}
	if tag {
		return a.commitFavoritesDeletes(ctx, nil, []int64{id})
	}
	return a.commitFavoritesDeletes(ctx, []int64{id}, nil)
}

func (a *App) undoFavoritesDelete(ctx context.Context, event DOMAction, tag bool) error {
	id, err := parsePositiveInt64(event.Data["id"], favoritesIDLabel(tag))
	if err != nil {
		return err
	}
	a.lockState()
	defer a.unlockState()
	pending := a.pendingFavoriteDeletes
	if tag {
		pending = a.pendingTagDeletes
	}
	if _, ok := pending[id]; !ok || a.closed {
		return nil
	}
	delete(pending, id)
	return a.refreshFavoritesRegionsLocked(ctx, true)
}

// commitFavoritesDeletes runs the service deletes for the given ids (all
// pending ones when both lists are nil). Successful commits render nothing:
// the items are already gone from the view and open edit panels stay open.
func (a *App) commitFavoritesDeletes(ctx context.Context, favoriteIDs, tagIDs []int64) error {
	a.lockState()
	all := favoriteIDs == nil && tagIDs == nil
	favorites := takePending(a.pendingFavoriteDeletes, favoriteIDs, all)
	tags := takePending(a.pendingTagDeletes, tagIDs, all)
	service := a.deps.Favorites
	a.unlockState()

	if service == nil || len(favorites)+len(tags) == 0 {
		return nil
	}
	errs := runConcurrently(len(favorites)+len(tags), func(i int) error {
		if i < len(favorites) {
			return service.DeleteFavorite(ctx, favorites[i])
		}
		return service.DeleteTag(ctx, tags[i-len(favorites)])
	})
	var failed []string
	for _, err := range errs {
		if err != nil {
			failed = append(failed, err.Error())
		}
	}
	if len(failed) == 0 {
		return nil
	}
	a.lockState()
	defer a.unlockState()
	if a.closed {
		return nil
	}
	a.favoritesError = "Could not delete: " + strings.Join(failed, "; ")
	return a.refreshFavoritesRegionsLocked(ctx, false)
}

// takePending removes ids (every pending id when all is set) from pending
// before the service call, so a late undo can no longer resurrect them.
func takePending(pending map[int64]string, ids []int64, all bool) []int64 {
	if all {
		ids = make([]int64, 0, len(pending))
		for id := range pending {
			ids = append(ids, id)
		}
		slices.Sort(ids) // stable error message order
	}
	taken := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := pending[id]; ok {
			delete(pending, id)
			taken = append(taken, id)
		}
	}
	return taken
}

func (a *App) favoriteLabelLocked(id int64) string {
	for _, favorite := range a.favoritesData.Favorites {
		if favorite != nil && int64(favorite.ID) == id {
			return favoriteItemLabel(favorite)
		}
	}
	return "favorite"
}

func (a *App) tagNameLocked(id int64) string {
	for _, tag := range a.favoritesData.Tags {
		if tag != nil && int64(tag.ID) == id {
			return tag.Name
		}
	}
	return "tag"
}
