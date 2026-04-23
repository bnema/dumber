package domainerrors

type domainError string

func (e domainError) Error() string {
	return string(e)
}

// ErrRelatedWebViewUnsupported reports that the active engine cannot create
// a popup WebView related to an existing parent WebView.
var ErrRelatedWebViewUnsupported error = domainError("related popup webviews are not supported")
