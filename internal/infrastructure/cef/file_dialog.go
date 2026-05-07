package cef

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"
	"github.com/bnema/puregotk/v4/gio"
	"github.com/bnema/puregotk/v4/gobject"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/rs/zerolog"

	"github.com/bnema/dumber/internal/logging"
)

type cefFileDialogRequest struct {
	Mode               purecef.FileDialogMode
	Title              string
	DefaultFilePath    string
	AcceptFilters      []string
	AcceptExtensions   []string
	AcceptDescriptions []string
}

type fileDialogPresenter func(*WebView, cefFileDialogRequest, purecef.FileDialogCallback)

const maxFolderUploadExperimentFiles = 5000

var (
	decodeCEFStringList   = purecef.StringListToSlice
	newCEFStringList      = purecef.NewStringList
	freeCEFStringList     = purecef.FreeStringList
	continueCEFFileDialog = continueCEFFileDialogWithLogs
)

func (h *handlerSet) OnFileDialog(
	_ purecef.Browser,
	mode purecef.FileDialogMode,
	title string,
	defaultFilePath string,
	acceptFilters purecef.StringList,
	acceptExtensions purecef.StringList,
	acceptDescriptions purecef.StringList,
	callback purecef.FileDialogCallback,
) int32 {
	if h == nil || h.wv == nil || callback == nil {
		return 0
	}

	presenter := h.fileDialogPresenter
	if presenter == nil {
		presenter = presentGTKFileDialog
	}

	req := cefFileDialogRequest{
		Mode:               mode,
		Title:              title,
		DefaultFilePath:    defaultFilePath,
		AcceptFilters:      decodeCEFStringList(acceptFilters),
		AcceptExtensions:   decodeCEFStringList(acceptExtensions),
		AcceptDescriptions: decodeCEFStringList(acceptDescriptions),
	}
	logCEFFileDialog(h.wv).Debug().
		Int32("mode", mode).
		Str("title", title).
		Str("default_file_path", defaultFilePath).
		Int("accept_filter_count", len(req.AcceptFilters)).
		Int("accept_extension_count", len(req.AcceptExtensions)).
		Msg("cef: file dialog requested")

	presenter(h.wv, req, callback)
	return 1
}

func presentGTKFileDialog(wv *WebView, req cefFileDialogRequest, callback purecef.FileDialogCallback) {
	if callback == nil || wv == nil {
		return
	}

	wv.runOnGTK(func() {
		logCEFFileDialog(wv).Debug().
			Int32("mode", req.Mode).
			Str("title", req.Title).
			Str("default_file_path", req.DefaultFilePath).
			Msg("cef: presenting GTK file dialog")
		dialog, ok := newNativeFileDialog(wv, req)
		if !ok || dialog == nil {
			dispatchFileDialogResult(wv, callback, nil)
			return
		}

		onResponse := func(_ gtk.NativeDialog, responseID int) {
			defer func() {
				logCEFFileDialog(wv).Debug().Int32("mode", req.Mode).Msg("cef: releasing GTK file dialog")
				dialog.Unref()
				logCEFFileDialog(wv).Debug().Int32("mode", req.Mode).Msg("cef: released GTK file dialog")
			}()

			normalizedResponseID := normalizeGTKResponseID(responseID)
			logCEFFileDialog(wv).Debug().
				Int("response_id_raw", responseID).
				Int("response_id_normalized", normalizedResponseID).
				Int32("mode", req.Mode).
				Msg("cef: GTK file dialog response received")
			if normalizedResponseID != int(gtk.ResponseAcceptValue) {
				logCEFFileDialog(wv).Debug().Msg("cef: file dialog cancelled")
				dispatchFileDialogResult(wv, callback, nil)
				return
			}

			paths := selectedFileDialogPaths(dialog, req.Mode)
			logCEFFileDialog(wv).Debug().
				Int("path_count", len(paths)).
				Strs("paths", paths).
				Msg("cef: file dialog selection captured")
			paths = folderUploadExperimentPaths(wv, req.Mode, paths)
			dispatchFileDialogResult(wv, callback, paths)
		}
		dialog.ConnectResponse(&onResponse)
		dialog.Show()
	})
}

func newNativeFileDialog(wv *WebView, req cefFileDialogRequest) (*gtk.FileChooserNative, bool) {
	action, ok := fileChooserActionForMode(req.Mode)
	if !ok {
		logCEFFileDialog(wv).Warn().Int32("mode", req.Mode).Msg("cef: unsupported file dialog mode")
		return nil, false
	}

	var titlePtr *string
	title := strings.TrimSpace(req.Title)
	if title != "" {
		titlePtr = &title
	}

	dialog := gtk.NewFileChooserNative(titlePtr, fileDialogParentWindow(wv), action, nil, nil)
	if dialog == nil {
		logCEFFileDialog(wv).Warn().Int32("mode", req.Mode).Msg("cef: failed to create GTK file chooser dialog")
		return nil, false
	}

	dialog.SetModal(true)
	if req.Mode == purecef.FileDialogModeFileDialogOpenMultiple {
		dialog.SetSelectMultiple(true)
	}
	applyDefaultDialogPath(dialog, req)
	applyDialogFilters(dialog, req)
	return dialog, true
}

func fileChooserActionForMode(mode purecef.FileDialogMode) (gtk.FileChooserAction, bool) {
	switch mode {
	case purecef.FileDialogModeFileDialogOpen, purecef.FileDialogModeFileDialogOpenMultiple:
		return gtk.FileChooserActionOpenValue, true
	case purecef.FileDialogModeFileDialogOpenFolder:
		return gtk.FileChooserActionSelectFolderValue, true
	case purecef.FileDialogModeFileDialogSave:
		return gtk.FileChooserActionSaveValue, true
	default:
		return 0, false
	}
}

func fileDialogParentWindow(wv *WebView) *gtk.Window {
	if wv == nil || wv.viewBridge == nil {
		return nil
	}
	widget := wv.viewBridge.Widget()
	if widget == nil {
		return nil
	}
	ancestor := widget.GetAncestor(gtk.WindowGLibType())
	if ancestor == nil {
		return nil
	}
	// puregotk's GetAncestor wrapper adds a reference before returning.
	defer ancestor.Unref()
	return gtk.WindowNewFromInternalPtr(ancestor.GoPointer())
}

func applyDefaultDialogPath(dialog *gtk.FileChooserNative, req cefFileDialogRequest) {
	if dialog == nil {
		return
	}
	path := strings.TrimSpace(req.DefaultFilePath)
	if path == "" {
		return
	}

	if req.Mode == purecef.FileDialogModeFileDialogSave {
		if dir := filepath.Dir(path); dir != "." && dir != "" {
			if folder := gio.FileNewForPath(dir); folder != nil {
				_, _ = dialog.SetCurrentFolder(folder)
			}
		}
		if name := filepath.Base(path); name != "." && name != string(filepath.Separator) {
			dialog.SetCurrentName(name)
		}
		return
	}

	if info, err := os.Stat(path); err == nil && info.IsDir() {
		if folder := gio.FileNewForPath(path); folder != nil {
			_, _ = dialog.SetCurrentFolder(folder)
		}
		return
	}

	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if folder := gio.FileNewForPath(dir); folder != nil {
			_, _ = dialog.SetCurrentFolder(folder)
		}
	}
	if req.Mode != purecef.FileDialogModeFileDialogOpenFolder {
		if file := gio.FileNewForPath(path); file != nil {
			_, _ = dialog.SetFile(file)
		}
	}
}

func applyDialogFilters(dialog *gtk.FileChooserNative, req cefFileDialogRequest) {
	if dialog == nil {
		return
	}

	appliedAny := false
	for i, spec := range req.AcceptFilters {
		filter := gtk.NewFileFilter()
		if filter == nil {
			continue
		}

		if name := dialogFilterName(req, i, spec); name != "" {
			filter.SetName(&name)
		}

		added := false
		for _, token := range dialogFilterTokens(spec, dialogFilterExtensions(req, i)) {
			if addDialogFilterToken(filter, token) {
				added = true
			}
		}
		if !added {
			continue
		}

		dialog.AddFilter(filter)
		if !appliedAny {
			dialog.SetFilter(filter)
			appliedAny = true
		}
	}
}

func dialogFilterName(req cefFileDialogRequest, index int, fallback string) string {
	if index < len(req.AcceptDescriptions) {
		if name := strings.TrimSpace(req.AcceptDescriptions[index]); name != "" {
			return name
		}
	}
	return strings.TrimSpace(fallback)
}

func dialogFilterExtensions(req cefFileDialogRequest, index int) string {
	if index >= len(req.AcceptExtensions) {
		return ""
	}
	return req.AcceptExtensions[index]
}

func dialogFilterTokens(spec, extensions string) []string {
	tokens := make([]string, 0, 4)
	if spec = strings.TrimSpace(spec); spec != "" {
		tokens = append(tokens, spec)
	}
	for _, part := range strings.Split(extensions, ";") {
		part = strings.TrimSpace(part)
		if part != "" {
			tokens = append(tokens, part)
		}
	}
	return tokens
}

func addDialogFilterToken(filter *gtk.FileFilter, token string) bool {
	if filter == nil {
		return false
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	if strings.Contains(token, "/") && !strings.HasPrefix(token, ".") {
		filter.AddMimeType(token)
		return true
	}
	if strings.HasPrefix(token, ".") {
		filter.AddPattern("*" + token)
		return true
	}
	if strings.ContainsRune(token, '*') {
		filter.AddPattern(token)
		return true
	}
	return false
}

func selectedFileDialogPaths(dialog *gtk.FileChooserNative, mode purecef.FileDialogMode) []string {
	if dialog == nil {
		return nil
	}
	if mode == purecef.FileDialogModeFileDialogOpenMultiple {
		model := dialog.GetFiles()
		if model == nil {
			return nil
		}
		defer unrefGObjectPointer(model.GoPointer())
		paths := make([]string, 0, model.GetNItems())
		for i := uint(0); i < model.GetNItems(); i++ {
			obj := model.GetObject(i)
			if obj == nil {
				continue
			}
			file := &gio.FileBase{Ptr: obj.GoPointer()}
			if path := gioFileDialogPath(file); path != "" {
				paths = append(paths, path)
			}
			obj.Unref()
		}
		return paths
	}
	file := dialog.GetFile()
	defer unrefGObjectPointer(file.GoPointer())
	if path := gioFileDialogPath(file); path != "" {
		return []string{path}
	}
	return nil
}

func folderUploadExperimentPaths(wv *WebView, mode purecef.FileDialogMode, paths []string) []string {
	if mode != purecef.FileDialogModeFileDialogOpenFolder || len(paths) != 1 {
		return paths
	}
	folder := strings.TrimSpace(paths[0])
	if folder == "" {
		return paths
	}
	info, err := os.Stat(folder)
	if err != nil || !info.IsDir() {
		logCEFFileDialog(wv).Warn().Err(err).Str("path", folder).Msg("cef: folder mode experiment skipped; selected path is not a directory")
		return paths
	}

	files := make([]string, 0, 64)
	truncated := false
	walkErr := filepath.WalkDir(folder, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			logCEFFileDialog(wv).Debug().Err(err).Str("path", path).Msg("cef: folder mode experiment skipped unreadable entry")
			return nil
		}
		if path == folder || entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		files = append(files, path)
		if len(files) >= maxFolderUploadExperimentFiles {
			truncated = true
			return filepath.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		logCEFFileDialog(wv).Warn().Err(walkErr).Str("folder", folder).Msg("cef: folder mode experiment failed to enumerate selected folder")
		return paths
	}
	if len(files) == 0 {
		logCEFFileDialog(wv).Warn().Str("folder", folder).Msg("cef: folder mode experiment found no descendant files; using selected directory path")
		return paths
	}

	logCEFFileDialog(wv).Warn().
		Str("folder", folder).
		Int("file_count", len(files)).
		Bool("truncated", truncated).
		Msg("cef: folder mode experiment passing descendant file paths instead of selected directory")
	return files
}

func gioFileDialogPath(file *gio.FileBase) string {
	if file == nil {
		return ""
	}
	return strings.TrimSpace(file.GetPath())
}

func unrefGObjectPointer(ptr uintptr) {
	if ptr == 0 {
		return
	}
	obj := &gobject.Object{}
	obj.SetGoPointer(ptr)
	obj.Unref()
}

func normalizeGTKResponseID(responseID int) int {
	return int(int32(uint32(responseID)))
}

func dispatchFileDialogResult(wv *WebView, callback purecef.FileDialogCallback, paths []string) {
	if callback == nil {
		return
	}
	if wv == nil || wv.engine == nil {
		return
	}

	task := cefNewTask(cefTaskFunc(func() {
		logger := logCEFFileDialog(wv)
		logger.Debug().
			Int("path_count", len(paths)).
			Strs("paths", paths).
			Int32("currently_on_ui", purecef.CurrentlyOn(purecef.ThreadIDTidUi)).
			Msg("cef: file dialog continuation task executing")
		continueCEFFileDialog(logger, callback, paths...)
		logger.Debug().
			Int("path_count", len(paths)).
			Strs("paths", paths).
			Msg("cef: file dialog continuation task finished")
	}))
	if result := cefPostTask(purecef.ThreadIDTidUi, task); result != 1 {
		logCEFFileDialog(wv).Warn().Int32("post_result", result).Msg("cef: failed to post file dialog continuation to CEF UI thread")
	} else {
		logCEFFileDialog(wv).Debug().Int("path_count", len(paths)).Strs("paths", paths).Msg("cef: file dialog continuation task posted")
	}
}

func continueCEFFileDialogWithLogs(logger *zerolog.Logger, callback purecef.FileDialogCallback, filePaths ...string) {
	logger = nonNilCEFFileDialogLogger(logger)
	callbackPtr := rawCEFPointer(callback)
	if callback == nil {
		logger.Debug().Msg("cef: file dialog callback continuation skipped; callback is nil")
		return
	}

	if len(filePaths) == 0 {
		logger.Debug().Uint64("callback_ptr", uint64(callbackPtr)).Msg("cef: invoking file dialog callback cancel")
		callback.Cancel()
		runtime.KeepAlive(callback)
		logger.Debug().Uint64("callback_ptr", uint64(callbackPtr)).Msg("cef: file dialog callback cancel returned")
		return
	}

	logger.Debug().
		Uint64("callback_ptr", uint64(callbackPtr)).
		Int("path_count", len(filePaths)).
		Strs("paths", filePaths).
		Msg("cef: allocating file dialog CEF string list")
	list := newCEFStringList(filePaths...)
	if list == 0 {
		logger.Warn().
			Uint64("callback_ptr", uint64(callbackPtr)).
			Int("path_count", len(filePaths)).
			Msg("cef: file dialog CEF string list allocation failed; canceling")
		callback.Cancel()
		runtime.KeepAlive(callback)
		logger.Debug().Uint64("callback_ptr", uint64(callbackPtr)).Msg("cef: file dialog callback cancel returned after allocation failure")
		return
	}

	listValue := uint64(list)
	logger.Debug().
		Uint64("callback_ptr", uint64(callbackPtr)).
		Uint64("string_list", listValue).
		Int("path_count", len(filePaths)).
		Msg("cef: file dialog CEF string list allocated")
	logger.Debug().
		Uint64("callback_ptr", uint64(callbackPtr)).
		Uint64("string_list", listValue).
		Int("path_count", len(filePaths)).
		Strs("paths", filePaths).
		Msg("cef: invoking file dialog callback continuation")
	callback.Cont(list)
	logger.Debug().
		Uint64("callback_ptr", uint64(callbackPtr)).
		Uint64("string_list", listValue).
		Int("path_count", len(filePaths)).
		Msg("cef: file dialog callback continuation returned")
	logger.Debug().Uint64("string_list", listValue).Msg("cef: freeing file dialog CEF string list")
	freeCEFStringList(list)
	logger.Debug().Uint64("string_list", listValue).Msg("cef: freed file dialog CEF string list")
	runtime.KeepAlive(callback)
}

func nonNilCEFFileDialogLogger(logger *zerolog.Logger) *zerolog.Logger {
	if logger != nil {
		return logger
	}
	return logging.FromContext(context.Background())
}

type cefRawPointerHolder interface {
	RawPointer() unsafe.Pointer
}

func rawCEFPointer(v any) uintptr {
	if v == nil {
		return 0
	}
	holder, ok := v.(cefRawPointerHolder)
	if !ok {
		return 0
	}
	return uintptr(holder.RawPointer())
}

func logCEFFileDialog(wv *WebView) *zerolog.Logger {
	if wv == nil || wv.ctx == nil {
		return logging.FromContext(context.Background())
	}
	return logging.FromContext(wv.ctx)
}
