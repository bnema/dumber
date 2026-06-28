package usecase

import (
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/bnema/dumber/internal/domain/entity"
)

const (
	favoriteSidebarExactMatchScore    = 100
	favoriteSidebarTitlePrefixScore   = 90
	favoriteSidebarHostPrefixScore    = 80
	favoriteSidebarTagNameScore       = 70
	favoriteSidebarTitleContainsScore = 60
	favoriteSidebarURLContainsScore   = 50
)

// FavoriteSidebarQuery describes the pure filtering inputs for the native favorites sidebar.
type FavoriteSidebarQuery struct {
	Text   string
	TagIDs []entity.TagID
}

// FilterFavoritesForSidebar filters, deduplicates, and ranks favorites for the native favorites sidebar.
func FilterFavoritesForSidebar(favorites []*entity.Favorite, query FavoriteSidebarQuery) []*entity.Favorite {
	text := strings.ToLower(strings.TrimSpace(query.Text))
	selectedTagIDs := favoriteSidebarSelectedTagSet(query.TagIDs)

	rows := make([]favoriteSidebarFilterRow, 0, len(favorites))
	seen := make(map[string]struct{}, len(favorites))
	for index, favorite := range favorites {
		if favorite == nil {
			continue
		}
		if len(selectedTagIDs) > 0 && !favoriteSidebarHasAnyTag(favorite, selectedTagIDs) {
			continue
		}

		score := 0
		if text != "" {
			var matched bool
			score, matched = favoriteSidebarMatchScore(favorite, text)
			if !matched {
				continue
			}
		}

		key := favoriteSidebarDedupeKey(favorite)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		rows = append(rows, favoriteSidebarFilterRow{
			favorite: favorite,
			index:    index,
			score:    score,
		})
	}

	if text != "" || len(selectedTagIDs) > 0 {
		sort.SliceStable(rows, func(i, j int) bool {
			left, right := rows[i], rows[j]
			if left.score != right.score {
				return left.score > right.score
			}
			if left.favorite.Position != right.favorite.Position {
				return left.favorite.Position < right.favorite.Position
			}
			if left.favorite.URL != right.favorite.URL {
				return left.favorite.URL < right.favorite.URL
			}
			return left.index < right.index
		})
	}

	result := make([]*entity.Favorite, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.favorite)
	}
	return result
}

type favoriteSidebarFilterRow struct {
	favorite *entity.Favorite
	index    int
	score    int
}

func favoriteSidebarSelectedTagSet(tagIDs []entity.TagID) map[entity.TagID]struct{} {
	selectedTagIDs := make(map[entity.TagID]struct{}, len(tagIDs))
	for _, tagID := range tagIDs {
		selectedTagIDs[tagID] = struct{}{}
	}
	return selectedTagIDs
}

func favoriteSidebarHasAnyTag(favorite *entity.Favorite, selectedTagIDs map[entity.TagID]struct{}) bool {
	for _, tag := range favorite.Tags {
		if _, ok := selectedTagIDs[tag.ID]; ok {
			return true
		}
	}
	return false
}

func favoriteSidebarDedupeKey(favorite *entity.Favorite) string {
	if favorite.ID != 0 {
		return "id:" + strconv.FormatInt(int64(favorite.ID), 10)
	}
	if favorite.URL == "" {
		return ""
	}
	return "url:" + favorite.URL
}

func favoriteSidebarMatchScore(favorite *entity.Favorite, text string) (int, bool) {
	title := strings.ToLower(favorite.Title)
	favoriteURL := strings.ToLower(favorite.URL)
	host := favoriteSidebarURLHost(favorite.URL)

	switch {
	case title == text || favoriteURL == text:
		return favoriteSidebarExactMatchScore, true
	case strings.HasPrefix(title, text):
		return favoriteSidebarTitlePrefixScore, true
	case favoriteSidebarHostOrDomainHasPrefix(host, text):
		return favoriteSidebarHostPrefixScore, true
	case favoriteSidebarTagNameContains(favorite, text):
		return favoriteSidebarTagNameScore, true
	case strings.Contains(title, text):
		return favoriteSidebarTitleContainsScore, true
	case strings.Contains(favoriteURL, text):
		return favoriteSidebarURLContainsScore, true
	default:
		return 0, false
	}
}

func favoriteSidebarTagNameContains(favorite *entity.Favorite, text string) bool {
	for _, tag := range favorite.Tags {
		if strings.Contains(strings.ToLower(tag.Name), text) {
			return true
		}
	}
	return false
}

func favoriteSidebarHostOrDomainHasPrefix(host, text string) bool {
	host = strings.TrimPrefix(host, "www.")
	for host != "" {
		if strings.HasPrefix(host, text) {
			return true
		}
		dot := strings.IndexByte(host, '.')
		if dot < 0 {
			return false
		}
		host = host[dot+1:]
	}
	return false
}

func favoriteSidebarURLHost(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}
