//go:build stress

package stress

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	keepArtifacts   = 3
	artifactLayout  = "20060102-150405"
	coreRetries     = 3
	coreRetryDelay  = 2 * time.Second
	coredumpSinceTS = "2006-01-02 15:04:05"
)

// collect copies failure evidence into dir and writes summary.txt.
func (r *stressRun) collect(dir string) {
	t := r.t
	pruneArtifacts(t, filepath.Dir(dir), keepArtifacts-1)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		t.Logf("artifacts dir: %v", err)
		return
	}
	copyTo := func(src, dst string) {
		if err := copyPath(src, filepath.Join(dir, dst)); err != nil {
			t.Logf("artifact %s: %v", dst, err)
		}
	}
	if r.sb.bin != "" {
		copyTo(r.sb.bin, "dumber")
	}
	copyTo(r.logsDir(), "logs")
	if r.proc != nil {
		if _, err := os.Stat(r.fatalPath()); err == nil {
			copyTo(r.fatalPath(), "fatal.txt")
		}
	}
	if r.lastShot != "" {
		copyTo(r.lastShot, "last-shot.png")
	}
	if r.logDir != "" {
		copyTo(filepath.Join(r.logDir, "stdout.txt"), "stdout.txt")
		copyTo(filepath.Join(r.logDir, "stderr.txt"), "stderr.txt")
	}
	if r.proc != nil {
		r.dumpCore(filepath.Join(dir, "core"))
	}
	var sum strings.Builder
	fmt.Fprintf(&sum, "seed=%d\nduration=%s\nactions=%d\nlast_action=%s\nfirst_paint_s=%.1f\npid=%d\nsandbox=%s\nstderr=%s\nfailures:\n",
		r.seed, r.duration, r.actions, r.last, r.firstPaint.Seconds(), r.pid(), r.sb.root,
		filepath.Join(r.logDir, "stderr.txt"))
	for _, f := range r.failures {
		fmt.Fprintf(&sum, "- %s\n", f)
	}
	if err := os.WriteFile(filepath.Join(dir, "summary.txt"), []byte(sum.String()), filePerm); err != nil {
		t.Logf("summary.txt: %v", err)
	}
	abs, _ := filepath.Abs(dir)
	t.Logf("artifacts: %s", abs)
}

// dumpCore extracts this run's core with coredumpctl (best effort). It matches
// on PID, executable and start time, and retries while the core is written.
func (r *stressRun) dumpCore(core string) {
	args := []string{
		"dump", "COREDUMP_PID=" + strconv.Itoa(r.pid()), "COREDUMP_EXE=" + r.sb.bin,
		"--since=" + r.start.Format(coredumpSinceTS), "-o", core,
	}
	var out []byte
	var err error
	for i := range coreRetries {
		if i > 0 {
			time.Sleep(coreRetryDelay)
		}
		if out, err = exec.Command("coredumpctl", args...).CombinedOutput(); err == nil {
			return
		}
	}
	r.t.Logf("coredumpctl dump %d: %v %s", r.pid(), err, strings.TrimSpace(string(out)))
}

// pruneArtifacts keeps only the keep newest run dirs under root (cores are
// large). Only names in the run-dir timestamp layout are considered, and since
// they are UTC timestamps, lexical order is age order.
func pruneArtifacts(t *testing.T, root string, keep int) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	var dirs []string
	for _, e := range entries {
		if _, perr := time.Parse(artifactLayout, e.Name()); e.IsDir() && perr == nil {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Strings(dirs)
	for len(dirs) > keep {
		old := filepath.Join(root, dirs[0])
		if err := os.RemoveAll(old); err != nil {
			t.Logf("prune %s: %v", old, err)
		}
		dirs = dirs[1:]
	}
}

// copyPath copies a file or a directory tree from src to dst.
func copyPath(src, dst string) error {
	st, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return copyFile(src, dst)
	}
	return filepath.WalkDir(src, func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, dirPerm)
		}
		return copyFile(p, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
