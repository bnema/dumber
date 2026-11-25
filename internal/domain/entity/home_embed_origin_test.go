package entity

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHomeEmbedOrigin(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"http://192.168.1.10:3000/d/x?y=1", "http://192.168.1.10:3000", true},
		{"HTTPS://Grafana.LAN/", "https://grafana.lan", true},
		{"https://grafana.lan:443/x", "https://grafana.lan", true},
		{"http://grafana.lan:80", "http://grafana.lan", true},
		{"http://grafana.lan:8080", "http://grafana.lan:8080", true},
		{"http://[::1]:8080/", "http://[::1]:8080", true},
		{"http://[::1]/", "http://[::1]", true},
		{"https://bücher.example/", "https://xn--bcher-kva.example", true},
		{"https://example.com./", "https://example.com", true},
		{"https://user:pw@nas.lan:5001/", "https://nas.lan:5001", true},
		{"ftp://nas.lan/", "", false},
		{"dumb://home", "", false},
		{"https://dumber.invalid/", "", false},
		{"https://x.dumber.invalid/", "", false},
		{"/relative", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := HomeEmbedOrigin(tc.raw)
		assert.Equal(t, tc.ok, ok, tc.raw)
		assert.Equal(t, tc.want, got, tc.raw)
	}
}

func TestHomeLayoutEmbedOrigins(t *testing.T) {
	t.Parallel()
	embed := func(u string) *HomeServiceWidget { return &HomeServiceWidget{Type: HomeServiceWidgetEmbed, URL: u} }
	l := HomeLayout{Groups: []HomeGroup{
		{Services: []HomeService{
			{ID: "a", Widget: embed("http://10.0.0.2:3000/a")},
			{ID: "b", Widget: embed("http://10.0.0.2:3000/b")},
			{ID: "c", Widget: &HomeServiceWidget{Type: HomeServiceWidgetAPI, URL: "http://10.0.0.9/api"}},
			{ID: "d"},
		}},
		{Services: []HomeService{
			{ID: "e", Widget: embed("https://dumber.invalid/")},
			{ID: "f", Widget: embed("https://grafana.lan/")},
		}},
	}}
	assert.Equal(t, []string{"http://10.0.0.2:3000", "https://grafana.lan"}, l.EmbedOrigins())
	assert.Empty(t, HomeLayout{}.EmbedOrigins())
}
