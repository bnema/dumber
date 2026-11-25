package systemviews

import (
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// searchTreeDOM is a treeDOM that runs the Home search like the browser
// adapter: it re-applies query to the rows of its tree on every
// ReapplyHomeSearch, keeping selected as the adapter does.
type searchTreeDOM struct {
	*treeDOM
	query    string
	selected string
	reapply  int
}

func newSearchTreeDOM(query string) *searchTreeDOM {
	return &searchTreeDOM{treeDOM: newTreeDOM(), query: query, selected: ""}
}

func (d *searchTreeDOM) ReapplyHomeSearch() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reapply++
	var items []homeQueryItem
	var marks []*html.Node
	for _, kind := range []string{"service", "row"} {
		walkHTML(d.root, func(n *html.Node) {
			if htmlAttr(n, "data-sv-home-item") != kind {
				return
			}
			mark := n
			for p := n.Parent; p != nil; p = p.Parent {
				if strings.Contains(" "+htmlAttr(p, "class")+" ", " sv-home-service ") {
					mark = p
					break
				}
			}
			items = append(items, homeQueryItem{
				Key: strconv.Itoa(len(items)), Name: htmlAttr(n, "data-sv-home-name"),
				Text: htmlAttr(n, "data-sv-home-text"), URL: htmlAttr(n, "data-sv-home-url"),
			})
			marks = append(marks, mark)
		})
	}
	view := buildHomeSearchView(homeSearchConfig{}, d.query, items, nil, d.selected)
	if view.SelectedIndex >= 0 {
		d.selected = view.Selectable[view.SelectedIndex]
	}
	matched := map[string]bool{}
	for _, key := range view.Result.Matches {
		matched[key] = true
	}
	for i, item := range items {
		setHTMLAttr(marks[i], "data-sv-home-hidden", view.Filtering && !matched[item.Key])
		setHTMLAttr(marks[i], "data-sv-home-selected", item.Key == view.Selected)
	}
}

// shellState reports whether the service row of id is hidden and selected.
func (d *searchTreeDOM) shellState(id string) (exists, hidden, selected bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	shell := d.find("data-sv-anchor", homeShellAnchor(id))
	if shell == nil {
		return false, false, false
	}
	return true, hasHTMLAttr(shell, "data-sv-home-hidden"), hasHTMLAttr(shell, "data-sv-home-selected")
}

func walkHTML(n *html.Node, fn func(*html.Node)) {
	if n.Type == html.ElementNode {
		fn(n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkHTML(c, fn)
	}
}

func htmlAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func hasHTMLAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

func setHTMLAttr(n *html.Node, key string, on bool) {
	attrs := n.Attr[:0]
	for _, a := range n.Attr {
		if a.Key != key {
			attrs = append(attrs, a)
		}
	}
	n.Attr = attrs
	if on {
		n.Attr = append(n.Attr, html.Attribute{Key: key})
	}
}
