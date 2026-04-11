package systemviews

import (
	"fmt"
	"html"
)

type DOM interface {
	Mount(html string) error
}

func placeholderHTML(route Route) string {
	name := html.EscapeString(string(route))
	if name == "" || route == RouteUnknown {
		name = "unknown"
	}

	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>systemviews</title>
</head>
<body>
  <main id="app" data-route="%s">systemviews %s</main>
</body>
</html>`, name, name)
}
