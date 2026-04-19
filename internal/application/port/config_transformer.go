package port

// ConfigTransformer transforms legacy config formats to current format.
type ConfigTransformer interface {
	// TransformLegacyActions converts old-format action bindings (slices)
	// to new ActionBinding format (map with keys and desc).
	// Modifies the rawConfig map in place.
	TransformLegacyActions(rawConfig map[string]any)
}
