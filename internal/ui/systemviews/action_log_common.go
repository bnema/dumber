package systemviews

func errorString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}
