package homeconfig

// StarterTemplate is written by EnsureExists. It must stay valid TOML and
// decode into the default layout.
const StarterTemplate = `# Dumber home page (dumb://home).
# Changes are applied when you save this file.

[settings]
title = "Home"
columns = 3 # 1..6

# Built-in widgets, shown at the top in this order.
# Types: search, clock, favorites, top_sites, recent.
[[widgets]]
type = "search"

[[widgets]]
type = "clock"
format = "24h" # or "12h"

[[widgets]]
type = "favorites"
limit = 8
# tag = "dev"

[[widgets]]
type = "top_sites"
limit = 8

[[widgets]]
type = "recent"
limit = 8

# Bookmark groups.
# [[bookmarks]]
# group = "Dev"
# links = [
#   { name = "Docs", url = "https://example.com/docs" },
# ]

# Service groups with optional live widgets and actions.
# [[groups]]
# name = "Homelab"
#
# [[groups.services]]
# id = "media"                  # [a-z0-9_-], unique
# name = "Media server"
# url = "https://media.local"
# description = "Movies and shows"
# icon = "icons/media.svg"      # relative to the config dir
#
# [groups.services.widget]
# type = "api"                  # api: JSON stats | embed: iframe (url + height)
# url = "https://media.local/api/stats"
# headers = { "X-Token" = "{{env.MEDIA_TOKEN}}" }
# refresh = "60s"
# display = "stats"             # stats | list | bar
# fields = [ { label = "Movies", path = "MovieCount", format = "number" } ]
# template = "<strong>{{ index .fields \"Movies\" }}</strong> movies"  # optional HTML, replaces display
#
# [[groups.services.actions]]
# id = "restart"
# label = "Restart"
# method = "POST"
# url = "https://media.local/api/restart"
# confirm = "Restart the media server?"
# inputs = [ { name = "reason", label = "Reason" } ]   # {{input.reason}} in url, or in a JSON body string
`
