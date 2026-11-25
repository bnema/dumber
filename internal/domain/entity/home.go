package entity

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
)

// HomeBuiltinWidgetType identifies a built-in widget shown at the top of dumb://home.
type HomeBuiltinWidgetType string

// Built-in widget types.
const (
	HomeWidgetClock     HomeBuiltinWidgetType = "clock"
	HomeWidgetSearch    HomeBuiltinWidgetType = "search"
	HomeWidgetFavorites HomeBuiltinWidgetType = "favorites"
	HomeWidgetTopSites  HomeBuiltinWidgetType = "top_sites"
	HomeWidgetRecent    HomeBuiltinWidgetType = "recent"
)

// HomeClockFormat is the clock widget display format.
type HomeClockFormat string

// Clock formats.
const (
	HomeClock24h HomeClockFormat = "24h"
	HomeClock12h HomeClockFormat = "12h"
)

// HomeServiceWidgetType identifies the widget attached to a service.
type HomeServiceWidgetType string

// Service widget types.
const (
	HomeServiceWidgetAPI   HomeServiceWidgetType = "api"
	HomeServiceWidgetEmbed HomeServiceWidgetType = "embed"
)

// HomeDisplay is how an api widget renders its fields.
type HomeDisplay string

// Api widget displays.
const (
	HomeDisplayStats HomeDisplay = "stats"
	HomeDisplayList  HomeDisplay = "list"
	HomeDisplayBar   HomeDisplay = "bar"
)

// HomeFieldFormat is how a field value is formatted.
type HomeFieldFormat string

// Field formats.
const (
	HomeFormatText     HomeFieldFormat = "text"
	HomeFormatNumber   HomeFieldFormat = "number"
	HomeFormatPercent  HomeFieldFormat = "percent"
	HomeFormatBytes    HomeFieldFormat = "bytes"
	HomeFormatDuration HomeFieldFormat = "duration"
)

// HomeMethod is the HTTP method of a service action.
type HomeMethod string

// Action methods.
const (
	HomeMethodGet    HomeMethod = "GET"
	HomeMethodPost   HomeMethod = "POST"
	HomeMethodPut    HomeMethod = "PUT"
	HomeMethodPatch  HomeMethod = "PATCH"
	HomeMethodDelete HomeMethod = "DELETE"
)

// Home layout defaults and limits.
const (
	HomeDefaultTitle   = "Home"
	HomeDefaultColumns = 3
	HomeMinColumns     = 1
	HomeMaxColumns     = 6
	HomeDefaultLimit   = 8
	HomeMinLimit       = 1
	HomeMaxLimit       = 50
	HomeDefaultHeight  = 240
	HomeMinHeight      = 80
	HomeMaxHeight      = 2000
	HomeDefaultRefresh = "60s"
	HomeMinRefresh     = 5 * time.Second
	HomeDefaultTimeout = "10s"
	HomeMaxTimeout     = 60 * time.Second
	// HomeEmbedBlockedHost is the internal page host that embeds must never target.
	HomeEmbedBlockedHost = "dumber.invalid"
	// HomeMaxTemplateLen bounds an api widget template, in bytes.
	HomeMaxTemplateLen = 16 * 1024
)

var (
	homeIDPattern        = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)
	homeInputNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
)

// HomeLayout is the full dumb://home configuration (home.toml).
type HomeLayout struct {
	Settings  HomeSettings        `toml:"settings" json:"settings"`
	Widgets   []HomeBuiltinWidget `toml:"widgets" json:"widgets"`
	Bookmarks []HomeBookmarkGroup `toml:"bookmarks" json:"bookmarks"`
	Groups    []HomeGroup         `toml:"groups" json:"groups"`
}

// HomeSettings holds page-level settings.
type HomeSettings struct {
	Title   string `toml:"title" json:"title"`
	Columns int    `toml:"columns" json:"columns"`
}

// HomeBuiltinWidget is a built-in widget at the top of the page.
type HomeBuiltinWidget struct {
	Type   HomeBuiltinWidgetType `toml:"type" json:"type"`
	Format HomeClockFormat       `toml:"format,omitempty" json:"format,omitempty"`
	Limit  int                   `toml:"limit,omitempty" json:"limit,omitempty"`
	Tag    string                `toml:"tag,omitempty" json:"tag,omitempty"`
}

// HomeBookmarkGroup is a named list of links.
type HomeBookmarkGroup struct {
	Group string     `toml:"group" json:"group"`
	Links []HomeLink `toml:"links" json:"links"`
}

// HomeLink is a bookmark link.
type HomeLink struct {
	Name string `toml:"name" json:"name"`
	URL  string `toml:"url" json:"url"`
}

// HomeGroup is a named group of services.
type HomeGroup struct {
	Name     string        `toml:"name" json:"name"`
	Services []HomeService `toml:"services" json:"services"`
}

// HomeService is a service card.
type HomeService struct {
	ID          string `toml:"id" json:"id"`
	Name        string `toml:"name" json:"name"`
	URL         string `toml:"url,omitempty" json:"url,omitempty"`
	Description string `toml:"description,omitempty" json:"description,omitempty"`
	Icon        string `toml:"icon,omitempty" json:"icon,omitempty"`
	// IconVersion changes when the service's icon may have changed (url,
	// icon path, override file mtime). Set natively for the page only; the
	// page appends it to /api/home-icon as a cache buster.
	IconVersion string             `toml:"-" json:"icon_version,omitempty"`
	Widget      *HomeServiceWidget `toml:"widget,omitempty" json:"widget,omitempty"`
	Actions     []HomeAction       `toml:"actions,omitempty" json:"actions,omitempty"`
}

// HomeServiceWidget is the live widget attached to a service.
type HomeServiceWidget struct {
	Type    HomeServiceWidgetType `toml:"type" json:"type"`
	URL     string                `toml:"url,omitempty" json:"url,omitempty"`
	Headers map[string]string     `toml:"headers,omitempty" json:"headers,omitempty"`
	Refresh string                `toml:"refresh,omitempty" json:"refresh,omitempty"`
	Display HomeDisplay           `toml:"display,omitempty" json:"display,omitempty"`
	// Items is the path to an array rendered one row per element when
	// display is list. Field paths are then relative to each element. Empty
	// with a top-level JSON array uses that array.
	Items string `toml:"items,omitempty" json:"items,omitempty"`
	// Template is an HTML template rendered natively and sanitized. When set,
	// Display is ignored. It is never sent to the page.
	Template string      `toml:"template,omitempty" json:"template,omitempty"`
	Fields   []HomeField `toml:"fields,omitempty" json:"fields,omitempty"`
	Height   int         `toml:"height,omitempty" json:"height,omitempty"`
	Timeout  string      `toml:"timeout,omitempty" json:"timeout,omitempty"`
}

// HomeField maps a JSON path of an api response to a displayed value.
type HomeField struct {
	Label  string          `toml:"label" json:"label"`
	Path   string          `toml:"path" json:"path"`
	Format HomeFieldFormat `toml:"format,omitempty" json:"format,omitempty"`
}

// HomeAction is a user-triggered HTTP request bound to a service.
type HomeAction struct {
	ID      string            `toml:"id" json:"id"`
	Label   string            `toml:"label" json:"label"`
	Method  HomeMethod        `toml:"method,omitempty" json:"method,omitempty"`
	URL     string            `toml:"url,omitempty" json:"url,omitempty"`
	Headers map[string]string `toml:"headers,omitempty" json:"headers,omitempty"`
	Body    string            `toml:"body,omitempty" json:"body,omitempty"`
	Confirm string            `toml:"confirm,omitempty" json:"confirm,omitempty"`
	Inputs  []HomeActionInput `toml:"inputs,omitempty" json:"inputs,omitempty"`
}

// HomeActionInput is a value the user types before running an action. The
// action url and body reference it as {{input.NAME}}.
type HomeActionInput struct {
	Name        string `toml:"name" json:"name"`
	Label       string `toml:"label" json:"label"`
	Placeholder string `toml:"placeholder,omitempty" json:"placeholder,omitempty"`
}

// InputNames returns the declared input names.
func (a HomeAction) InputNames() []string {
	names := make([]string, 0, len(a.Inputs))
	for _, in := range a.Inputs {
		names = append(names, in.Name)
	}
	return names
}

// HomeValidationError describes one invalid value in a home layout.
type HomeValidationError struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e HomeValidationError) Error() string {
	return e.Path + ": " + e.Message
}

// DefaultHomeLayout returns the layout used when no home.toml exists.
func DefaultHomeLayout() HomeLayout {
	return HomeLayout{
		Settings: HomeSettings{Title: HomeDefaultTitle, Columns: HomeDefaultColumns},
		Widgets: []HomeBuiltinWidget{
			{Type: HomeWidgetSearch},
			{Type: HomeWidgetClock, Format: HomeClock24h},
			{Type: HomeWidgetFavorites, Limit: HomeDefaultLimit},
			{Type: HomeWidgetTopSites, Limit: HomeDefaultLimit},
			{Type: HomeWidgetRecent, Limit: HomeDefaultLimit},
		},
	}
}

// RefreshDuration parses Refresh. It returns 0 if Refresh is invalid.
func (w HomeServiceWidget) RefreshDuration() time.Duration {
	d, _ := time.ParseDuration(w.Refresh)
	return d
}

// TimeoutDuration parses Timeout. It returns 0 if Timeout is invalid.
func (w HomeServiceWidget) TimeoutDuration() time.Duration {
	d, _ := time.ParseDuration(w.Timeout)
	return d
}

// Normalize trims values and applies defaults in place.
func (l *HomeLayout) Normalize() {
	l.Settings.Title = strings.TrimSpace(l.Settings.Title)
	if l.Settings.Title == "" {
		l.Settings.Title = HomeDefaultTitle
	}
	if l.Settings.Columns == 0 {
		l.Settings.Columns = HomeDefaultColumns
	}
	for i := range l.Widgets {
		normalizeBuiltinWidget(&l.Widgets[i])
	}
	for i := range l.Bookmarks {
		b := &l.Bookmarks[i]
		b.Group = strings.TrimSpace(b.Group)
		for j := range b.Links {
			b.Links[j].Name = strings.TrimSpace(b.Links[j].Name)
			b.Links[j].URL = strings.TrimSpace(b.Links[j].URL)
		}
	}
	for i := range l.Groups {
		g := &l.Groups[i]
		g.Name = strings.TrimSpace(g.Name)
		for j := range g.Services {
			normalizeService(&g.Services[j])
		}
	}
}

func normalizeBuiltinWidget(w *HomeBuiltinWidget) {
	w.Type = HomeBuiltinWidgetType(strings.TrimSpace(string(w.Type)))
	w.Format = HomeClockFormat(strings.TrimSpace(string(w.Format)))
	w.Tag = strings.TrimSpace(w.Tag)
	switch w.Type {
	case HomeWidgetClock:
		if w.Format == "" {
			w.Format = HomeClock24h
		}
	case HomeWidgetFavorites, HomeWidgetTopSites, HomeWidgetRecent:
		if w.Limit == 0 {
			w.Limit = HomeDefaultLimit
		}
	case HomeWidgetSearch:
	}
}

func normalizeService(s *HomeService) {
	s.ID = strings.TrimSpace(s.ID)
	s.Name = strings.TrimSpace(s.Name)
	s.URL = strings.TrimSpace(s.URL)
	s.Description = strings.TrimSpace(s.Description)
	s.Icon = strings.TrimSpace(s.Icon)
	if w := s.Widget; w != nil {
		w.Type = HomeServiceWidgetType(strings.TrimSpace(string(w.Type)))
		w.URL = strings.TrimSpace(w.URL)
		w.Refresh = strings.TrimSpace(w.Refresh)
		w.Timeout = strings.TrimSpace(w.Timeout)
		w.Display = HomeDisplay(strings.TrimSpace(string(w.Display)))
		w.Items = strings.TrimSpace(w.Items)
		for k := range w.Fields {
			f := &w.Fields[k]
			f.Label = strings.TrimSpace(f.Label)
			f.Path = strings.TrimSpace(f.Path)
			f.Format = HomeFieldFormat(strings.TrimSpace(string(f.Format)))
			if f.Format == "" {
				f.Format = HomeFormatText
			}
		}
		switch w.Type {
		case HomeServiceWidgetAPI:
			if w.Refresh == "" {
				w.Refresh = HomeDefaultRefresh
			}
			if w.Timeout == "" {
				w.Timeout = HomeDefaultTimeout
			}
			if w.Display == "" {
				w.Display = HomeDisplayStats
			}
		case HomeServiceWidgetEmbed:
			if w.Height == 0 {
				w.Height = HomeDefaultHeight
			}
		}
	}
	for k := range s.Actions {
		a := &s.Actions[k]
		a.ID = strings.TrimSpace(a.ID)
		a.Label = strings.TrimSpace(a.Label)
		a.URL = strings.TrimSpace(a.URL)
		a.Confirm = strings.TrimSpace(a.Confirm)
		for j := range a.Inputs {
			in := &a.Inputs[j]
			in.Name = strings.TrimSpace(in.Name)
			in.Label = strings.TrimSpace(in.Label)
			in.Placeholder = strings.TrimSpace(in.Placeholder)
		}
		a.Method = HomeMethod(strings.ToUpper(strings.TrimSpace(string(a.Method))))
		if a.Method == "" {
			a.Method = HomeMethodPost
		}
	}
}

type homeErrs []HomeValidationError

func (e *homeErrs) addf(p, format string, args ...any) {
	*e = append(*e, HomeValidationError{Path: p, Message: fmt.Sprintf(format, args...)})
}

// Validate checks a normalized layout and returns every error found.
func (l HomeLayout) Validate() []HomeValidationError {
	var errs homeErrs
	if l.Settings.Columns < HomeMinColumns || l.Settings.Columns > HomeMaxColumns {
		errs.addf("settings.columns", "must be between %d and %d", HomeMinColumns, HomeMaxColumns)
	}
	for i, w := range l.Widgets {
		validateBuiltinWidget(&errs, fmt.Sprintf("widgets[%d]", i), w)
	}
	for i, b := range l.Bookmarks {
		p := fmt.Sprintf("bookmarks[%d]", i)
		if b.Group == "" {
			errs.addf(p+".group", "is required")
		}
		for j, link := range b.Links {
			lp := fmt.Sprintf("%s.links[%d]", p, j)
			if link.Name == "" {
				errs.addf(lp+".name", "is required")
			}
			if !isHTTPURL(link.URL) {
				errs.addf(lp+".url", "must be an http(s) URL")
			}
		}
	}
	seen := map[string]string{}
	for i, g := range l.Groups {
		p := fmt.Sprintf("groups[%d]", i)
		if g.Name == "" {
			errs.addf(p+".name", "is required")
		}
		for j := range g.Services {
			s := &g.Services[j]
			sp := fmt.Sprintf("%s.services[%d]", p, j)
			if s.ID != "" {
				if first, dup := seen[s.ID]; dup {
					errs.addf(sp+".id", "duplicate id %q (already used by %s)", s.ID, first)
				} else {
					seen[s.ID] = sp
				}
			}
			validateService(&errs, sp, *s)
		}
	}
	return errs
}

func validateBuiltinWidget(errs *homeErrs, p string, w HomeBuiltinWidget) {
	switch w.Type {
	case HomeWidgetClock:
		if w.Format != HomeClock24h && w.Format != HomeClock12h {
			errs.addf(p+".format", "must be 24h or 12h")
		}
	case HomeWidgetFavorites, HomeWidgetTopSites, HomeWidgetRecent:
		if w.Limit < HomeMinLimit || w.Limit > HomeMaxLimit {
			errs.addf(p+".limit", "must be between %d and %d", HomeMinLimit, HomeMaxLimit)
		}
	case HomeWidgetSearch:
	default:
		errs.addf(p+".type", "unknown widget type %q", w.Type)
	}
}

func validateService(errs *homeErrs, p string, s HomeService) {
	if !homeIDPattern.MatchString(s.ID) {
		errs.addf(p+".id", "must match [a-z0-9_-]{1,64}")
	}
	if s.Name == "" {
		errs.addf(p+".name", "is required")
	}
	if s.URL != "" && !isHTTPURL(s.URL) {
		errs.addf(p+".url", "must be an http(s) URL")
	}
	if s.Icon != "" {
		if msg := checkIconPath(s.Icon); msg != "" {
			errs.addf(p+".icon", "%s", msg)
		}
	}
	if s.Widget != nil {
		validateServiceWidget(errs, p+".widget", *s.Widget)
	}
	actionIDs := map[string]bool{}
	for i := range s.Actions {
		a := &s.Actions[i]
		ap := fmt.Sprintf("%s.actions[%d]", p, i)
		if !homeIDPattern.MatchString(a.ID) {
			errs.addf(ap+".id", "must match [a-z0-9_-]{1,64}")
		} else if actionIDs[a.ID] {
			errs.addf(ap+".id", "duplicate action id %q", a.ID)
		}
		actionIDs[a.ID] = true
		validateActionInputs(errs, ap, a)
		if a.Label == "" {
			errs.addf(ap+".label", "is required")
		}
		switch a.Method {
		case HomeMethodGet, HomeMethodPost, HomeMethodPut, HomeMethodPatch, HomeMethodDelete:
		default:
			errs.addf(ap+".method", "must be GET, POST, PUT, PATCH or DELETE")
		}
		if !isHTTPURL(a.URL) {
			errs.addf(ap+".url", "must be an http(s) URL")
		}
	}
}

func validateServiceWidget(errs *homeErrs, p string, w HomeServiceWidget) {
	switch w.Type {
	case HomeServiceWidgetAPI:
		if !isHTTPURL(w.URL) {
			errs.addf(p+".url", "must be an http(s) URL")
		}
		if d, err := time.ParseDuration(w.Refresh); err != nil {
			errs.addf(p+".refresh", "invalid duration %q", w.Refresh)
		} else if d < HomeMinRefresh {
			errs.addf(p+".refresh", "must be at least %s", HomeMinRefresh)
		}
		if d, err := time.ParseDuration(w.Timeout); err != nil {
			errs.addf(p+".timeout", "invalid duration %q", w.Timeout)
		} else if d <= 0 || d > HomeMaxTimeout {
			errs.addf(p+".timeout", "must be greater than 0 and at most %s", HomeMaxTimeout)
		}
		switch w.Display {
		case HomeDisplayStats, HomeDisplayList, HomeDisplayBar:
		default:
			errs.addf(p+".display", "must be stats, list or bar")
		}
		if w.Items != "" && w.Display != HomeDisplayList {
			errs.addf(p+".items", "is only allowed with display = list")
		}
		if len(w.Template) > HomeMaxTemplateLen {
			errs.addf(p+".template", "must be at most %d bytes", HomeMaxTemplateLen)
		}
		validateHomeFields(errs, p, w.Fields, w.Template != "")
	case HomeServiceWidgetEmbed:
		if w.Template != "" {
			errs.addf(p+".template", "is only allowed for api widgets")
		}
		if !isHTTPURL(w.URL) {
			errs.addf(p+".url", "must be an http(s) URL")
		} else if IsBlockedEmbedURL(w.URL) {
			errs.addf(p+".url", "host %q is not allowed", HomeEmbedBlockedHost)
		}
		if len(w.Headers) > 0 {
			errs.addf(p+".headers", "is only allowed for api widgets")
		}
		if w.Height < HomeMinHeight || w.Height > HomeMaxHeight {
			errs.addf(p+".height", "must be between %d and %d", HomeMinHeight, HomeMaxHeight)
		}
	default:
		errs.addf(p+".type", "must be api or embed")
	}
}

func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func checkIconPath(icon string) string {
	if strings.HasPrefix(icon, "/") || strings.Contains(icon, "\\") || strings.Contains(icon, ":") {
		return "must be a relative path"
	}
	for _, part := range strings.Split(icon, "/") {
		if part == ".." {
			return "must not contain .."
		}
	}
	switch strings.ToLower(path.Ext(icon)) {
	case ".svg", ".png", ".ico", ".webp":
		return ""
	default:
		return "extension must be .svg, .png, .ico or .webp"
	}
}

// FindService returns the service with the given id.
func (l HomeLayout) FindService(id string) (HomeService, bool) {
	for _, g := range l.Groups {
		for i := range g.Services {
			if g.Services[i].ID == id {
				return g.Services[i], true
			}
		}
	}
	return HomeService{}, false
}

// FindAction returns the action with the given id.
func (s HomeService) FindAction(id string) (HomeAction, bool) {
	for i := range s.Actions {
		if s.Actions[i].ID == id {
			return s.Actions[i], true
		}
	}
	return HomeAction{}, false
}

// RedactedForPage returns a deep copy safe to send to the page: no headers,
// no action bodies or URLs, and no api widget URL.
func (l HomeLayout) RedactedForPage() HomeLayout {
	out := HomeLayout{
		Settings:  l.Settings,
		Widgets:   append([]HomeBuiltinWidget(nil), l.Widgets...),
		Bookmarks: make([]HomeBookmarkGroup, len(l.Bookmarks)),
		Groups:    make([]HomeGroup, len(l.Groups)),
	}
	for i, b := range l.Bookmarks {
		out.Bookmarks[i] = HomeBookmarkGroup{Group: b.Group, Links: append([]HomeLink(nil), b.Links...)}
	}
	for i, g := range l.Groups {
		services := make([]HomeService, len(g.Services))
		for j := range g.Services {
			services[j] = redactService(g.Services[j])
		}
		out.Groups[i] = HomeGroup{Name: g.Name, Services: services}
	}
	return out
}

func redactService(s HomeService) HomeService {
	r := HomeService{ID: s.ID, Name: s.Name, URL: s.URL, Description: s.Description, Icon: s.Icon}
	if w := s.Widget; w != nil {
		rw := &HomeServiceWidget{
			Type:    w.Type,
			Display: w.Display,
			Items:   w.Items,
			Fields:  append([]HomeField(nil), w.Fields...),
			Refresh: w.Refresh,
			Height:  w.Height,
		}
		if w.Type == HomeServiceWidgetEmbed {
			rw.URL = w.URL
		}
		r.Widget = rw
	}
	for i := range s.Actions {
		a := &s.Actions[i]
		r.Actions = append(r.Actions, HomeAction{
			ID: a.ID, Label: a.Label, Confirm: a.Confirm, Method: a.Method,
			Inputs: append([]HomeActionInput(nil), a.Inputs...),
		})
	}
	return r
}

func validateActionInputs(errs *homeErrs, p string, a *HomeAction) {
	names := map[string]bool{}
	for i, in := range a.Inputs {
		ip := fmt.Sprintf("%s.inputs[%d]", p, i)
		if !homeInputNamePattern.MatchString(in.Name) {
			errs.addf(ip+".name", "must match [A-Za-z_][A-Za-z0-9_]{0,63}")
		} else if names[in.Name] {
			errs.addf(ip+".name", "duplicate input name %q", in.Name)
		}
		names[in.Name] = true
		if in.Label == "" {
			errs.addf(ip+".label", "is required")
		}
	}
	for _, name := range HomeInputRefs(a.URL) {
		if !names[name] {
			errs.addf(p+".url", "references undeclared input %q", name)
		}
	}
	if HomeInputsInURLOrigin(a.URL) {
		errs.addf(p+".url", "inputs are only allowed in the path, query or fragment, not in the scheme, host or port")
	}
	for _, name := range HomeInputRefs(a.Body) {
		if !names[name] {
			errs.addf(p+".body", "references undeclared input %q", name)
		}
	}
	if len(HomeInputRefs(a.Body)) > 0 {
		if msg := CheckHomeInputBody(a.Body); msg != "" {
			errs.addf(p+".body", "%s", msg)
		}
	}
	for _, k := range sortedKeys(a.Headers) {
		if len(HomeInputRefs(a.Headers[k])) > 0 {
			errs.addf(p+".headers."+k, "must not reference inputs")
		}
	}
}

// HomeInputRefs returns the input names referenced as {{input.NAME}} in s,
// in order of first appearance.
func HomeInputRefs(s string) []string {
	var out []string
	for _, m := range homeInputPattern.FindAllStringSubmatch(s, -1) {
		if !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func validateHomeFields(errs *homeErrs, p string, fields []HomeField, hasTemplate bool) {
	if len(fields) == 0 && !hasTemplate {
		errs.addf(p+".fields", "at least one field is required")
	}
	for i, f := range fields {
		fp := fmt.Sprintf("%s.fields[%d]", p, i)
		if f.Label == "" {
			errs.addf(fp+".label", "is required")
		}
		if f.Path == "" {
			errs.addf(fp+".path", "is required")
		}
		switch f.Format {
		case HomeFormatText, HomeFormatNumber, HomeFormatPercent, HomeFormatBytes, HomeFormatDuration:
		default:
			errs.addf(fp+".format", "must be text, number, percent, bytes or duration")
		}
	}
}
