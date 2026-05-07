package cef

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type stubFileDialogCallback struct {
	list     purecef.StringList
	paths    []string
	canceled bool
}

func (s *stubFileDialogCallback) Cont(filePaths purecef.StringList) {
	s.list = filePaths
	s.paths = decodeCEFStringList(filePaths)
}

func (s *stubFileDialogCallback) Cancel() {
	s.canceled = true
}

func TestGetDialogHandlerEnabled(t *testing.T) {
	h := &handlerSet{}
	require.Same(t, h, h.GetDialogHandler())
}

func TestOnFileDialogRejectsNilWebViewOrCallback(t *testing.T) {
	require.Zero(t, (*handlerSet)(nil).OnFileDialog(nil, purecef.FileDialogModeFileDialogOpen, "", "", 0, 0, 0, &stubFileDialogCallback{}))
	require.Zero(t, (&handlerSet{}).OnFileDialog(nil, purecef.FileDialogModeFileDialogOpen, "", "", 0, 0, 0, &stubFileDialogCallback{}))
	require.Zero(t, (&handlerSet{wv: &WebView{ctx: context.Background()}}).OnFileDialog(nil, purecef.FileDialogModeFileDialogOpen, "", "", 0, 0, 0, nil))
}

func TestNormalizeGTKResponseID(t *testing.T) {
	require.Equal(t, -3, normalizeGTKResponseID(4294967293))
	require.Equal(t, -6, normalizeGTKResponseID(4294967290))
	require.Equal(t, 0, normalizeGTKResponseID(0))
}

func TestOnFileDialogDelegatesDecodedRequestToPresenter(t *testing.T) {
	prevDecode := decodeCEFStringList
	decodeCEFStringList = func(list purecef.StringList) []string {
		switch list {
		case purecef.StringList(11):
			return []string{"image/*", ".txt"}
		case purecef.StringList(12):
			return []string{".png;.jpg", ".txt"}
		case purecef.StringList(13):
			return []string{"Image Files", "Text Files"}
		default:
			return nil
		}
	}
	defer func() { decodeCEFStringList = prevDecode }()

	var gotReq cefFileDialogRequest
	var gotCalls int
	h := &handlerSet{
		wv: &WebView{ctx: context.Background()},
		fileDialogPresenter: func(_ *WebView, req cefFileDialogRequest, _ purecef.FileDialogCallback) {
			gotReq = req
			gotCalls++
		},
	}

	handled := h.OnFileDialog(nil, purecef.FileDialogModeFileDialogOpenMultiple, "Pick files", "/tmp/start.txt", purecef.StringList(11), purecef.StringList(12), purecef.StringList(13), &stubFileDialogCallback{})

	require.Equal(t, int32(1), handled)
	require.Equal(t, 1, gotCalls)
	require.Equal(t, purecef.FileDialogModeFileDialogOpenMultiple, gotReq.Mode)
	require.Equal(t, "Pick files", gotReq.Title)
	require.Equal(t, "/tmp/start.txt", gotReq.DefaultFilePath)
	require.Equal(t, []string{"image/*", ".txt"}, gotReq.AcceptFilters)
	require.Equal(t, []string{".png;.jpg", ".txt"}, gotReq.AcceptExtensions)
	require.Equal(t, []string{"Image Files", "Text Files"}, gotReq.AcceptDescriptions)
}

func TestFolderUploadExperimentPathsExpandsDirectoryFiles(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.txt")
	second := filepath.Join(dir, "nested", "second.txt")
	require.NoError(t, os.MkdirAll(filepath.Dir(second), 0o755))
	require.NoError(t, os.WriteFile(first, []byte("first"), 0o644))
	require.NoError(t, os.WriteFile(second, []byte("second"), 0o644))

	got := folderUploadExperimentPaths(&WebView{ctx: context.Background()}, purecef.FileDialogModeFileDialogOpenFolder, []string{dir})

	require.ElementsMatch(t, []string{first, second}, got)
}

func TestFolderUploadExperimentPathsKeepsEmptyDirectoryPath(t *testing.T) {
	dir := t.TempDir()

	got := folderUploadExperimentPaths(&WebView{ctx: context.Background()}, purecef.FileDialogModeFileDialogOpenFolder, []string{dir})

	require.Equal(t, []string{dir}, got)
}

func TestOnFileDialogPresenterCanContinueCallback(t *testing.T) {
	prevDecode := decodeCEFStringList
	decodeCEFStringList = func(purecef.StringList) []string { return nil }
	defer func() { decodeCEFStringList = prevDecode }()

	prevContinue := continueCEFFileDialog
	continueCEFFileDialog = func(_ *zerolog.Logger, callback purecef.FileDialogCallback, filePaths ...string) {
		recorder, ok := callback.(*stubFileDialogCallback)
		require.True(t, ok)
		recorder.paths = append([]string(nil), filePaths...)
	}
	defer func() { continueCEFFileDialog = prevContinue }()

	callback := &stubFileDialogCallback{}
	h := &handlerSet{
		wv: &WebView{ctx: context.Background()},
		fileDialogPresenter: func(_ *WebView, _ cefFileDialogRequest, cb purecef.FileDialogCallback) {
			continueCEFFileDialog(nil, cb, "/tmp/report.txt")
		},
	}

	handled := h.OnFileDialog(nil, purecef.FileDialogModeFileDialogSave, "", "", 0, 0, 0, callback)

	require.Equal(t, int32(1), handled)
	require.False(t, callback.canceled)
	require.Equal(t, []string{"/tmp/report.txt"}, callback.paths)
}
