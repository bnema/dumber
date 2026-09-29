package homepage

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// Config holds dependencies for homepage handlers.
type Config struct {
	HistoryUC   port.HomepageHistory
	FavoritesUC port.HomepageFavorites
}

// RegisterHandlers registers all homepage message handlers with the router.
func RegisterHandlers(ctx context.Context, router port.WebUIHandlerRouter, cfg Config) error {
	log := logging.FromContext(ctx)
	log.Debug().Msg("registering homepage message handlers")

	handlers := Handlers(cfg)
	for msgType, handler := range handlers {
		if err := port.RegisterWebUIMessage(router, msgType, handler); err != nil {
			return fmt.Errorf("failed to register handler %s: %w", msgType, err)
		}
		log.Debug().Str("type", msgType).Msg("registered homepage handler")
	}

	log.Info().Int("count", len(handlers)).Msg("homepage handlers registered")
	return nil
}

// Handlers returns every homepage handler keyed by its WebUI message type.
func Handlers(cfg Config) map[string]port.WebUIMessageHandler {
	history := NewHistoryHandlers(cfg.HistoryUC)
	favorites := NewFavoritesHandlers(cfg.FavoritesUC)
	tags := NewTagHandlers(cfg.FavoritesUC)

	return map[string]port.WebUIMessageHandler{
		port.MsgHistoryTimeline:         history.HandleTimeline(),
		port.MsgHistoryTimelineByDomain: history.HandleTimelineByDomain(),
		port.MsgHistoryTimelineWindow:   history.HandleTimelineWindow(),
		port.MsgHistorySearchFTS:        history.HandleSearchFTS(),
		port.MsgHistoryDeleteEntry:      history.HandleDeleteEntry(),
		port.MsgHistoryDeleteRange:      history.HandleDeleteRange(),
		port.MsgHistoryClearAll:         history.HandleClearAll(),
		port.MsgHistoryStats:            history.HandleStats(),
		port.MsgHistoryAnalytics:        history.HandleAnalytics(),
		port.MsgHistoryDomainStats:      history.HandleDomainStats(),
		port.MsgHistoryDeleteDomain:     history.HandleDeleteDomain(),

		port.MsgFavoriteList:          favorites.HandleList(),
		port.MsgFavoriteCreate:        favorites.HandleCreate(),
		port.MsgFavoriteUpdate:        favorites.HandleUpdate(),
		port.MsgFavoriteDelete:        favorites.HandleDelete(),
		port.MsgFavoriteSetShortcut:   favorites.HandleSetShortcut(),
		port.MsgFavoriteGetByShortcut: favorites.HandleGetByShortcut(),

		port.MsgTagList:   tags.HandleList(),
		port.MsgTagCreate: tags.HandleCreate(),
		port.MsgTagDelete: tags.HandleDelete(),
		port.MsgTagUpdate: tags.HandleUpdate(),
		port.MsgTagAssign: tags.HandleAssign(),
		port.MsgTagRemove: tags.HandleRemove(),
	}
}
