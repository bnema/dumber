package cef

import (
	"context"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
	cefmocks "github.com/bnema/purego-cef/cef/mocks"
	"github.com/stretchr/testify/require"
)

func TestWebViewRunJavaScript_PostsExecutionToCefUIThread(t *testing.T) {
	browser := cefmocks.NewMockBrowser(t)
	frame := cefmocks.NewMockFrame(t)
	const script = "document.body.dataset.dumber = '1'"

	oldTask := cefNewTask
	oldPost := cefPostTask
	defer func() {
		cefNewTask = oldTask
		cefPostTask = oldPost
	}()
	cefNewTask = func(task purecef.Task) purecef.Task { return task }

	var scheduled purecef.Task
	cefPostTask = func(threadID purecef.ThreadID, task purecef.Task) int32 {
		require.Equal(t, purecef.ThreadIDTidUi, threadID)
		require.NotNil(t, task)
		scheduled = task
		return 1
	}

	wv := &WebView{ctx: context.Background(), engine: &Engine{}, browser: browser}
	wv.RunJavaScript(context.Background(), script)

	require.NotNil(t, scheduled)
	browser.EXPECT().GetMainFrame().Return(frame).Once()
	frame.EXPECT().ExecuteJavaScript(script, "", int32(0)).Once()
	scheduled.Execute()
}
