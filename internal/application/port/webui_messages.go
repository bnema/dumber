package port

// WebUI message types exchanged between internal pages and Go handlers.
// Both the handler registries and the systemviews bridge use these names,
// so a rename or a new message only has to be declared here.
const (
	MsgHistoryTimeline         = "history_timeline"
	MsgHistoryTimelineByDomain = "history_timeline_by_domain"
	MsgHistoryTimelineWindow   = "history_timeline_window"
	MsgHistorySearchFTS        = "history_search_fts"
	MsgHistoryDeleteEntry      = "history_delete_entry"
	MsgHistoryDeleteRange      = "history_delete_range"
	MsgHistoryClearAll         = "history_clear_all"
	MsgHistoryStats            = "history_stats"
	MsgHistoryAnalytics        = "history_analytics"
	MsgHistoryDomainStats      = "history_domain_stats"
	MsgHistoryDeleteDomain     = "history_delete_domain"

	MsgFavoriteList          = "favorite_list"
	MsgFavoriteCreate        = "favorite_create"
	MsgFavoriteUpdate        = "favorite_update"
	MsgFavoriteDelete        = "favorite_delete"
	MsgFavoriteSetShortcut   = "favorite_set_shortcut"
	MsgFavoriteGetByShortcut = "favorite_get_by_shortcut"

	MsgTagList   = "tag_list"
	MsgTagCreate = "tag_create"
	MsgTagDelete = "tag_delete"
	MsgTagUpdate = "tag_update"
	MsgTagAssign = "tag_assign"
	MsgTagRemove = "tag_remove"

	MsgSaveConfig          = "save_config"
	MsgGetKeybindings      = "get_keybindings"
	MsgSetKeybinding       = "set_keybinding"
	MsgResetKeybinding     = "reset_keybinding"
	MsgResetAllKeybindings = "reset_all_keybindings"
)

// WebUICallbacks names the JS functions that receive a message's result.
type WebUICallbacks struct {
	Success string
	Failure string
}

// Homepage callbacks shared by every history, favorite and tag message.
var homepageCallbacks = WebUICallbacks{Success: "__dumber_homepage_response", Failure: "__dumber_error"}

// WebUIMessageCallbacks is the single source of truth for which JS callbacks
// answer each WebUI message. Handlers register with these callbacks and the
// systemviews bridge waits on them.
var WebUIMessageCallbacks = map[string]WebUICallbacks{
	MsgHistoryTimeline:         homepageCallbacks,
	MsgHistoryTimelineByDomain: homepageCallbacks,
	MsgHistoryTimelineWindow:   homepageCallbacks,
	MsgHistorySearchFTS:        homepageCallbacks,
	MsgHistoryDeleteEntry:      homepageCallbacks,
	MsgHistoryDeleteRange:      homepageCallbacks,
	MsgHistoryClearAll:         homepageCallbacks,
	MsgHistoryStats:            homepageCallbacks,
	MsgHistoryAnalytics:        homepageCallbacks,
	MsgHistoryDomainStats:      homepageCallbacks,
	MsgHistoryDeleteDomain:     homepageCallbacks,

	MsgFavoriteList:          homepageCallbacks,
	MsgFavoriteCreate:        homepageCallbacks,
	MsgFavoriteUpdate:        homepageCallbacks,
	MsgFavoriteDelete:        homepageCallbacks,
	MsgFavoriteSetShortcut:   homepageCallbacks,
	MsgFavoriteGetByShortcut: homepageCallbacks,

	MsgTagList:   homepageCallbacks,
	MsgTagCreate: homepageCallbacks,
	MsgTagDelete: homepageCallbacks,
	MsgTagUpdate: homepageCallbacks,
	MsgTagAssign: homepageCallbacks,
	MsgTagRemove: homepageCallbacks,

	MsgSaveConfig:          {Success: "__dumber_config_saved", Failure: "__dumber_config_error"},
	MsgGetKeybindings:      {Success: "__dumber_keybindings_loaded", Failure: "__dumber_keybindings_error"},
	MsgSetKeybinding:       {Success: "__dumber_keybinding_set", Failure: "__dumber_keybinding_set_error"},
	MsgResetKeybinding:     {Success: "__dumber_keybinding_reset", Failure: "__dumber_keybinding_reset_error"},
	MsgResetAllKeybindings: {Success: "__dumber_keybindings_reset_all", Failure: "__dumber_keybindings_reset_all_error"},
}

// RegisterWebUIMessage registers handler for msgType using the callbacks
// declared in WebUIMessageCallbacks, in the main world.
func RegisterWebUIMessage(router WebUIHandlerRouter, msgType string, handler WebUIMessageHandler) error {
	cb, ok := WebUIMessageCallbacks[msgType]
	if !ok {
		return &UnknownWebUIMessageError{Type: msgType}
	}
	return router.RegisterHandlerWithCallbacks(msgType, cb.Success, cb.Failure, "", handler)
}

// UnknownWebUIMessageError reports a message type missing from the contract.
type UnknownWebUIMessageError struct{ Type string }

func (e *UnknownWebUIMessageError) Error() string {
	return "unknown webui message type: " + e.Type
}
