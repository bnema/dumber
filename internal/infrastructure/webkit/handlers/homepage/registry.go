package homepage

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/webkit"
	"github.com/bnema/dumber/internal/logging"
)

// Config holds dependencies for homepage handlers.
type Config struct {
	HistoryUC   *usecase.SearchHistoryUseCase
	FavoritesUC *usecase.ManageFavoritesUseCase
}

// RegisterHandlers registers all homepage message handlers with the router.
func RegisterHandlers(ctx context.Context, router *webkit.MessageRouter, cfg Config) error {
	log := logging.FromContext(ctx)
	log.Debug().Msg("registering homepage message handlers")

	// Common callback for all homepage handlers
	const callback = "__dumber_homepage_response"
	const errorCallback = "__dumber_error"
	const worldName = "" // main world

	handlers := make(map[string]webkit.MessageHandler)

	// History handlers
	historyHandlers := NewHistoryHandlers(cfg.HistoryUC)
	handlers["history_timeline"] = historyHandlers.HandleTimeline()
	handlers["history_search_fts"] = historyHandlers.HandleSearchFTS()
	handlers["history_delete_entry"] = historyHandlers.HandleDeleteEntry()
	handlers["history_delete_range"] = historyHandlers.HandleDeleteRange()
	handlers["history_clear_all"] = historyHandlers.HandleClearAll()
	handlers["history_analytics"] = historyHandlers.HandleAnalytics()
	handlers["history_domain_stats"] = historyHandlers.HandleDomainStats()
	handlers["history_delete_domain"] = historyHandlers.HandleDeleteDomain()

	// Favorites handlers
	favoritesHandlers := NewFavoritesHandlers(cfg.FavoritesUC)
	handlers["favorite_list"] = favoritesHandlers.HandleList()
	handlers["favorite_set_shortcut"] = favoritesHandlers.HandleSetShortcut()
	handlers["favorite_get_by_shortcut"] = favoritesHandlers.HandleGetByShortcut()
	handlers["favorite_set_folder"] = favoritesHandlers.HandleSetFolder()

	// Folder handlers
	folderHandlers := NewFolderHandlers(cfg.FavoritesUC)
	handlers["folder_list"] = folderHandlers.HandleList()
	handlers["folder_create"] = folderHandlers.HandleCreate()
	handlers["folder_delete"] = folderHandlers.HandleDelete()
	handlers["folder_update"] = folderHandlers.HandleUpdate() // Requires UpdateFolder()

	// Tag handlers
	tagHandlers := NewTagHandlers(cfg.FavoritesUC)
	handlers["tag_list"] = tagHandlers.HandleList()
	handlers["tag_create"] = tagHandlers.HandleCreate()
	handlers["tag_delete"] = tagHandlers.HandleDelete()
	handlers["tag_update"] = tagHandlers.HandleUpdate() // Requires UpdateTag()
	handlers["tag_assign"] = tagHandlers.HandleAssign()
	handlers["tag_remove"] = tagHandlers.HandleRemove()

	// Register all handlers
	for msgType, handler := range handlers {
		if err := router.RegisterHandlerWithCallbacks(msgType, callback, errorCallback, worldName, handler); err != nil {
			return fmt.Errorf("failed to register handler %s: %w", msgType, err)
		}
		log.Debug().Str("type", msgType).Msg("registered homepage handler")
	}

	log.Info().Int("count", len(handlers)).Msg("homepage handlers registered")
	return nil
}
