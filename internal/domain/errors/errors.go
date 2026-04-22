package domainerrors

import "errors"

// ErrRelatedWebViewUnsupported reports that the active engine cannot create
// a popup WebView related to an existing parent WebView.
var ErrRelatedWebViewUnsupported = errors.New("related popup webviews are not supported")
