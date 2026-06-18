package entity

// TouchpadNavigationAction indicates the direction of a touchpad navigation gesture.
type TouchpadNavigationAction int

const (
	// TouchpadNavigationBack indicates a backward navigation gesture.
	TouchpadNavigationBack TouchpadNavigationAction = iota
	// TouchpadNavigationForward indicates a forward navigation gesture.
	TouchpadNavigationForward
)

// String returns a human-readable representation of the navigation action.
func (a TouchpadNavigationAction) String() string {
	switch a {
	case TouchpadNavigationBack:
		return "back"
	case TouchpadNavigationForward:
		return "forward"
	default:
		return "unknown"
	}
}

// TouchpadNavigationGesture describes visual progress for a deliberate
// two-finger history navigation gesture.
type TouchpadNavigationGesture struct {
	Action TouchpadNavigationAction
	// Progress is normalized gesture progress from 0.0 to 1.0.
	Progress         float64
	ThresholdReached bool
	Active           bool
}
