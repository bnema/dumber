package systemviews

import (
	"fmt"
	"strings"
	"sync"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// treeDOM is a fake DOM that keeps a real node tree, so tests can check that
// an element survived a reload (same node) and that edits only target
// elements that exist, like the browser adapter.
type treeDOM struct {
	mu      sync.Mutex
	root    *html.Node
	mounts  int
	swapped []string
	inserts int
	removes int
	styles  map[string]string
	// insertErr makes InsertAnchorMarkup fail; toastOK makes ShowToast
	// report a shown toast, recorded in toasts.
	insertErr error
	toastOK   bool
	toasts    []string
}

func (d *treeDOM) ShowToast(request ToastRequest) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.toastOK {
		d.toasts = append(d.toasts, request.Message)
	}
	return d.toastOK
}

func newTreeDOM() *treeDOM {
	return &treeDOM{root: &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}}
}

func (d *treeDOM) Mount(markup string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.mounts++
	return d.setChildren(d.root, markup)
}

func (d *treeDOM) setChildren(parent *html.Node, markup string) error {
	for child := parent.FirstChild; child != nil; {
		next := child.NextSibling
		parent.RemoveChild(child)
		child = next
	}
	nodes, err := html.ParseFragment(strings.NewReader(markup), parent)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		parent.AppendChild(node)
	}
	return nil
}

func (d *treeDOM) UpdateRegions(update RegionUpdate) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, region := range update.Regions {
		if d.find("data-sv-region", region.Name) == nil {
			return fmt.Errorf("systemview region %q not found", region.Name)
		}
	}
	for _, region := range update.Regions {
		d.swapped = append(d.swapped, region.Name)
		if err := d.setChildren(d.find("data-sv-region", region.Name), region.HTML); err != nil {
			return err
		}
	}
	return nil
}

func (d *treeDOM) InsertAnchorMarkup(parent, before, markup string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.insertErr != nil {
		return d.insertErr
	}
	container := d.find("data-sv-anchor", parent)
	if container == nil {
		return fmt.Errorf("anchor %q not found", parent)
	}
	var next *html.Node
	if before != "" {
		if next = d.find("data-sv-anchor", before); next == nil || next.Parent != container {
			return fmt.Errorf("anchor %q not a child of %q", before, parent)
		}
	}
	nodes, err := html.ParseFragment(strings.NewReader(markup), container)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		container.InsertBefore(node, next)
	}
	d.inserts++
	return nil
}

func (d *treeDOM) RemoveAnchor(anchor string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if node := d.find("data-sv-anchor", anchor); node != nil {
		node.Parent.RemoveChild(node)
		d.removes++
	}
	return nil
}

func (d *treeDOM) SetAnchorStyleProperty(anchor, property, value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.find("data-sv-anchor", anchor) == nil {
		return fmt.Errorf("anchor %q not found", anchor)
	}
	if d.styles == nil {
		d.styles = make(map[string]string)
	}
	d.styles[anchor+" "+property] = value
	return nil
}

// node returns the element with attr=value, or nil.
func (d *treeDOM) node(attr, value string) *html.Node {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.find(attr, value)
}

func (d *treeDOM) find(attr, value string) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key == attr && a.Val == value {
					found = n
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(d.root)
	return found
}

// text returns the text content of a region.
func (d *treeDOM) text(region string) string {
	d.mu.Lock()
	defer d.mu.Unlock()
	node := d.find("data-sv-region", region)
	if node == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(node)
	return strings.TrimSpace(b.String())
}

// html renders the mounted tree.
func (d *treeDOM) html() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var b strings.Builder
	for c := d.root.FirstChild; c != nil; c = c.NextSibling {
		_ = html.Render(&b, c)
	}
	return b.String()
}
