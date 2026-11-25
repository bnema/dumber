package port

// HomeEmbedOriginSetter receives the origins (scheme://host[:port]) of the
// embeds shown on dumb://home. An engine that grants local network access
// to system views implements it, to keep that access to these origins.
type HomeEmbedOriginSetter interface {
	SetHomeEmbedOrigins(origins []string)
}
