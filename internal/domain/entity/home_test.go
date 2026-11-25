package entity

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func validHomeLayout() HomeLayout {
	return HomeLayout{
		Settings: HomeSettings{Title: "Home", Columns: 3},
		Widgets:  []HomeBuiltinWidget{{Type: HomeWidgetClock}, {Type: HomeWidgetFavorites, Tag: "dev"}},
		Bookmarks: []HomeBookmarkGroup{{
			Group: "Dev",
			Links: []HomeLink{{Name: "ermite", URL: "https://git.bnema.dev"}},
		}},
		Groups: []HomeGroup{{
			Name: "Homelab",
			Services: []HomeService{
				{
					ID: "jellyfin", Name: "Jellyfin", URL: "https://jelly.local", Icon: "icons/jellyfin.svg",
					Widget: &HomeServiceWidget{
						Type:    HomeServiceWidgetAPI,
						URL:     "https://jelly.local/api?token=SECRETQUERY",
						Headers: map[string]string{"X-Token": "SECRETHEADER"},
						Fields:  []HomeField{{Label: "Films", Path: "MovieCount", Format: HomeFormatNumber}},
					},
					Actions: []HomeAction{{
						ID: "restart", Label: "Restart", Method: "post", URL: "https://jelly.local/restart",
						Headers: map[string]string{"Authorization": "SECRETACTION"}, Body: "SECRETBODY", Confirm: "Sure?",
					}},
				},
				{
					ID: "grafana", Name: "Grafana",
					Widget: &HomeServiceWidget{Type: HomeServiceWidgetEmbed, URL: "https://grafana.local/d/x"},
				},
			},
		}},
	}
}

func TestHomeNormalizeDefaults(t *testing.T) {
	l := validHomeLayout()
	l.Settings = HomeSettings{Title: "  "}
	l.Normalize()

	if l.Settings.Title != "Home" || l.Settings.Columns != 3 {
		t.Fatalf("settings defaults: %+v", l.Settings)
	}
	if l.Widgets[0].Format != HomeClock24h {
		t.Errorf("clock format = %q", l.Widgets[0].Format)
	}
	if l.Widgets[1].Limit != 8 {
		t.Errorf("favorites limit = %d", l.Widgets[1].Limit)
	}
	api := l.Groups[0].Services[0].Widget
	if api.Refresh != "60s" || api.Timeout != "10s" || api.Display != HomeDisplayStats {
		t.Errorf("api defaults: %+v", api)
	}
	if api.RefreshDuration() != time.Minute || api.TimeoutDuration() != 10*time.Second {
		t.Errorf("durations: %v %v", api.RefreshDuration(), api.TimeoutDuration())
	}
	if l.Groups[0].Services[0].Actions[0].Method != HomeMethodPost {
		t.Errorf("method = %q", l.Groups[0].Services[0].Actions[0].Method)
	}
	if l.Groups[0].Services[1].Widget.Height != 240 {
		t.Errorf("embed height = %d", l.Groups[0].Services[1].Widget.Height)
	}
	if errs := l.Validate(); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestHomeNormalizeFieldFormatAndMethodCase(t *testing.T) {
	l := validHomeLayout()
	l.Groups[0].Services[0].Widget.Fields[0].Format = ""
	l.Groups[0].Services[0].Actions[0].Method = " delete "
	l.Normalize()
	if got := l.Groups[0].Services[0].Widget.Fields[0].Format; got != HomeFormatText {
		t.Errorf("field format = %q", got)
	}
	if got := l.Groups[0].Services[0].Actions[0].Method; got != HomeMethodDelete {
		t.Errorf("method = %q", got)
	}
}

func TestDefaultHomeLayout(t *testing.T) {
	l := DefaultHomeLayout()
	want := []HomeBuiltinWidgetType{HomeWidgetSearch, HomeWidgetClock, HomeWidgetFavorites, HomeWidgetTopSites, HomeWidgetRecent}
	if len(l.Widgets) != len(want) {
		t.Fatalf("widgets = %+v", l.Widgets)
	}
	for i, w := range want {
		if l.Widgets[i].Type != w {
			t.Errorf("widget %d = %q, want %q", i, l.Widgets[i].Type, w)
		}
	}
	if l.Settings.Title != "Home" || len(l.Groups) != 0 {
		t.Errorf("unexpected default: %+v", l)
	}
	if errs := l.Validate(); len(errs) != 0 {
		t.Errorf("default layout invalid: %v", errs)
	}
}

func TestHomeValidate(t *testing.T) {
	svc := func(l *HomeLayout) *HomeService { return &l.Groups[0].Services[0] }
	tests := []struct {
		name   string
		mutate func(l *HomeLayout)
		path   string
	}{
		{"columns too high", func(l *HomeLayout) { l.Settings.Columns = 7 }, "settings.columns"},
		{"columns negative", func(l *HomeLayout) { l.Settings.Columns = -1 }, "settings.columns"},
		{"unknown widget", func(l *HomeLayout) { l.Widgets[0].Type = "weather" }, "widgets[0].type"},
		{"bad clock format", func(l *HomeLayout) { l.Widgets[0].Format = "36h" }, "widgets[0].format"},
		{"limit too high", func(l *HomeLayout) { l.Widgets[1].Limit = 51 }, "widgets[1].limit"},
		{"bookmark group", func(l *HomeLayout) { l.Bookmarks[0].Group = "" }, "bookmarks[0].group"},
		{"bookmark name", func(l *HomeLayout) { l.Bookmarks[0].Links[0].Name = "" }, "bookmarks[0].links[0].name"},
		{"bookmark url scheme", func(l *HomeLayout) { l.Bookmarks[0].Links[0].URL = "javascript:alert(1)" }, "bookmarks[0].links[0].url"},
		{"group name", func(l *HomeLayout) { l.Groups[0].Name = "" }, "groups[0].name"},
		{"service id missing", func(l *HomeLayout) { svc(l).ID = "" }, "groups[0].services[0].id"},
		{"service id charset", func(l *HomeLayout) { svc(l).ID = "Jelly Fin" }, "groups[0].services[0].id"},
		{"service id too long", func(l *HomeLayout) { svc(l).ID = strings.Repeat("a", 65) }, "groups[0].services[0].id"},
		{"service name", func(l *HomeLayout) { svc(l).Name = "" }, "groups[0].services[0].name"},
		{"service url", func(l *HomeLayout) { svc(l).URL = "file:///etc/passwd" }, "groups[0].services[0].url"},
		{"icon absolute", func(l *HomeLayout) { svc(l).Icon = "/etc/x.svg" }, "groups[0].services[0].icon"},
		{"icon dotdot", func(l *HomeLayout) { svc(l).Icon = "icons/../../x.svg" }, "groups[0].services[0].icon"},
		{"icon ext", func(l *HomeLayout) { svc(l).Icon = "icons/x.exe" }, "groups[0].services[0].icon"},
		{"widget type", func(l *HomeLayout) { svc(l).Widget.Type = "template" }, "groups[0].services[0].widget.type"},
		{"api url", func(l *HomeLayout) { svc(l).Widget.URL = "" }, "groups[0].services[0].widget.url"},
		{"api refresh min", func(l *HomeLayout) { svc(l).Widget.Refresh = "1s" }, "groups[0].services[0].widget.refresh"},
		{"api refresh parse", func(l *HomeLayout) { svc(l).Widget.Refresh = "soon" }, "groups[0].services[0].widget.refresh"},
		{"api timeout max", func(l *HomeLayout) { svc(l).Widget.Timeout = "61s" }, "groups[0].services[0].widget.timeout"},
		{"api display", func(l *HomeLayout) { svc(l).Widget.Display = "pie" }, "groups[0].services[0].widget.display"},
		{"api no fields", func(l *HomeLayout) { svc(l).Widget.Fields = nil }, "groups[0].services[0].widget.fields"},
		{"field label", func(l *HomeLayout) { svc(l).Widget.Fields[0].Label = "" }, "groups[0].services[0].widget.fields[0].label"},
		{"field path", func(l *HomeLayout) { svc(l).Widget.Fields[0].Path = "" }, "groups[0].services[0].widget.fields[0].path"},
		{"field format", func(l *HomeLayout) { svc(l).Widget.Fields[0].Format = "hex" }, "groups[0].services[0].widget.fields[0].format"},
		{"embed blocked host", func(l *HomeLayout) {
			l.Groups[0].Services[1].Widget.URL = "https://Dumber.invalid/x"
		}, "groups[0].services[1].widget.url"},
		{"embed height", func(l *HomeLayout) { l.Groups[0].Services[1].Widget.Height = 79 }, "groups[0].services[1].widget.height"},
		{"embed headers", func(l *HomeLayout) {
			l.Groups[0].Services[1].Widget.Headers = map[string]string{"a": "b"}
		}, "groups[0].services[1].widget.headers"},
		{"action id", func(l *HomeLayout) { svc(l).Actions[0].ID = "" }, "groups[0].services[0].actions[0].id"},
		{"action label", func(l *HomeLayout) { svc(l).Actions[0].Label = "" }, "groups[0].services[0].actions[0].label"},
		{"action method", func(l *HomeLayout) { svc(l).Actions[0].Method = "TRACE" }, "groups[0].services[0].actions[0].method"},
		{"action url", func(l *HomeLayout) { svc(l).Actions[0].URL = "ftp://x" }, "groups[0].services[0].actions[0].url"},
		{"duplicate action id", func(l *HomeLayout) {
			svc(l).Actions = append(svc(l).Actions, svc(l).Actions[0])
		}, "groups[0].services[0].actions[1].id"},
		{"duplicate service id", func(l *HomeLayout) {
			l.Groups = append(l.Groups, HomeGroup{Name: "Other", Services: []HomeService{{ID: "jellyfin", Name: "J2"}}})
		}, "groups[1].services[0].id"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := validHomeLayout()
			l.Normalize()
			tt.mutate(&l)
			errs := l.Validate()
			if len(errs) != 1 || errs[0].Path != tt.path {
				t.Fatalf("errors = %v, want one at %s", errs, tt.path)
			}
			if !strings.HasPrefix(errs[0].Error(), tt.path+": ") {
				t.Errorf("Error() = %q", errs[0].Error())
			}
		})
	}
}

func TestHomeValidateCollectsAllErrors(t *testing.T) {
	l := validHomeLayout()
	l.Normalize()
	l.Settings.Columns = 0
	l.Groups[0].Services[0].Name = ""
	l.Groups[0].Services[0].Actions[0].URL = ""
	if errs := l.Validate(); len(errs) != 3 {
		t.Fatalf("got %d errors, want 3: %v", len(errs), errs)
	}
}

func TestHomeRedactedForPage(t *testing.T) {
	l := validHomeLayout()
	l.Normalize()
	r := l.RedactedForPage()

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, secret := range []string{"SECRETHEADER", "SECRETACTION", "SECRETBODY", "SECRETQUERY", "jelly.local/restart", "headers", "body"} {
		if strings.Contains(out, secret) {
			t.Errorf("redacted json contains %q: %s", secret, out)
		}
	}
	if r.Groups[0].Services[1].Widget.URL != "https://grafana.local/d/x" {
		t.Errorf("embed url dropped")
	}
	a := r.Groups[0].Services[0].Actions[0]
	if a.ID != "restart" || a.Label != "Restart" || a.Method != HomeMethodPost || a.Confirm != "Sure?" || a.URL != "" || a.Body != "" || a.Headers != nil {
		t.Errorf("action = %+v", a)
	}
	w := r.Groups[0].Services[0].Widget
	if w.Display != HomeDisplayStats || w.Refresh != "60s" || len(w.Fields) != 1 {
		t.Errorf("widget = %+v", w)
	}
	// Original must be untouched.
	if l.Groups[0].Services[0].Widget.Headers["X-Token"] != "SECRETHEADER" || l.Groups[0].Services[0].Actions[0].Body == "" {
		t.Errorf("original layout mutated")
	}
}

func TestHomeLookups(t *testing.T) {
	l := validHomeLayout()
	s, ok := l.FindService("jellyfin")
	if !ok || s.Name != "Jellyfin" {
		t.Fatalf("FindService = %+v, %v", s, ok)
	}
	if _, found := l.FindService("nope"); found {
		t.Error("found missing service")
	}
	a, ok := s.FindAction("restart")
	if !ok || a.Label != "Restart" {
		t.Fatalf("FindAction = %+v, %v", a, ok)
	}
	if _, found := s.FindAction("nope"); found {
		t.Error("found missing action")
	}
}

func TestHomeValidateActionInputs(t *testing.T) {
	act := func(l *HomeLayout) *HomeAction { return &l.Groups[0].Services[0].Actions[0] }
	tests := []struct {
		name   string
		mutate func(a *HomeAction)
		path   string
	}{
		{"valid", func(a *HomeAction) {
			a.URL = "https://jelly.local/q?x={{input.q}}"
			a.Body = `{"q":"{{ input.q }}"}`
		}, ""},
		{"bad name", func(a *HomeAction) { a.Inputs[0].Name = "1q" }, "groups[0].services[0].actions[0].inputs[0].name"},
		{"duplicate name", func(a *HomeAction) {
			a.Inputs = append(a.Inputs, HomeActionInput{Name: "q", Label: "Again"})
		}, "groups[0].services[0].actions[0].inputs[1].name"},
		{"missing label", func(a *HomeAction) { a.Inputs[0].Label = "" }, "groups[0].services[0].actions[0].inputs[0].label"},
		{"undeclared in url", func(a *HomeAction) { a.URL = "https://jelly.local/{{input.zz}}" }, "groups[0].services[0].actions[0].url"},
		{"undeclared in body", func(a *HomeAction) { a.Body = `{"q":"{{input.zz}}"}` }, "groups[0].services[0].actions[0].body"},
		{"input in raw body", func(a *HomeAction) { a.Body = "text={{input.q}}" }, "groups[0].services[0].actions[0].body"},
		{"input outside JSON string", func(a *HomeAction) { a.Body = `{"n": {{input.q}}}` }, "groups[0].services[0].actions[0].body"},
		{"input body not JSON", func(a *HomeAction) { a.Body = `{"q": "{{input.q}}",}` }, "groups[0].services[0].actions[0].body"},
		{"input in nested JSON string with env", func(a *HomeAction) {
			a.Body = `[{"q":"pre {{input.q}} \" post","t":{{env.N}}}]`
		}, ""},
		{"raw body without inputs", func(a *HomeAction) { a.Body = "plain {{env.X}}" }, ""},
		{"input in header", func(a *HomeAction) {
			a.Headers = map[string]string{"X": "{{input.q}}"}
		}, "groups[0].services[0].actions[0].headers.X"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := validHomeLayout()
			act(&l).Inputs = []HomeActionInput{{Name: "q", Label: "Query"}}
			l.Normalize()
			tt.mutate(act(&l))
			errs := l.Validate()
			if tt.path == "" {
				if len(errs) != 0 {
					t.Fatalf("unexpected errors: %v", errs)
				}
				return
			}
			if len(errs) != 1 || errs[0].Path != tt.path {
				t.Fatalf("errors = %v, want one at %s", errs, tt.path)
			}
		})
	}
}

func TestHomeValidateItemsOnlyForList(t *testing.T) {
	l := validHomeLayout()
	l.Normalize()
	w := l.Groups[0].Services[0].Widget
	w.Items = "rows"
	if errs := l.Validate(); len(errs) != 1 || errs[0].Path != "groups[0].services[0].widget.items" {
		t.Fatalf("errors = %v", errs)
	}
	w.Display = HomeDisplayList
	if errs := l.Validate(); len(errs) != 0 {
		t.Fatalf("errors = %v", errs)
	}
}

func TestHomeRedactedKeepsInputs(t *testing.T) {
	l := validHomeLayout()
	l.Groups[0].Services[0].Actions[0].Inputs = []HomeActionInput{{Name: "q", Label: "Query", Placeholder: "p"}}
	l.Normalize()
	r := l.RedactedForPage()
	if got := r.Groups[0].Services[0].Actions[0].Inputs; len(got) != 1 || got[0].Placeholder != "p" {
		t.Fatalf("inputs = %+v", got)
	}
}

func TestHomeValidateTemplate(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(l *HomeLayout)
		path   string
	}{
		{"api template without fields", func(l *HomeLayout) {
			w := l.Groups[0].Services[0].Widget
			w.Template = "<b>{{.data.x}}</b>"
			w.Fields = nil
		}, ""},
		{"template too long", func(l *HomeLayout) {
			l.Groups[0].Services[0].Widget.Template = strings.Repeat("x", HomeMaxTemplateLen+1)
		}, "groups[0].services[0].widget.template"},
		{"template on embed", func(l *HomeLayout) {
			l.Groups[0].Services[1].Widget.Template = "<b>x</b>"
		}, "groups[0].services[1].widget.template"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := validHomeLayout()
			l.Normalize()
			tt.mutate(&l)
			errs := l.Validate()
			if tt.path == "" {
				if len(errs) != 0 {
					t.Fatalf("unexpected errors: %v", errs)
				}
				return
			}
			if len(errs) != 1 || errs[0].Path != tt.path {
				t.Fatalf("errors = %v, want one at %s", errs, tt.path)
			}
		})
	}
}

func TestHomeRedactedClearsTemplate(t *testing.T) {
	l := validHomeLayout()
	l.Groups[0].Services[0].Widget.Template = "<i>TEMPLATESECRET</i>"
	l.Normalize()
	data, err := json.Marshal(l.RedactedForPage())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "TEMPLATESECRET") || strings.Contains(string(data), `"template"`) {
		t.Fatalf("template leaked: %s", data)
	}
}
