//go:build stress

package stress

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	shotRetries    = 10
	startupRetries = 40
	slowFirstPaint = 10 * time.Second
	shotRetryDelay = time.Second
	shotMinRatio   = 0.20
	shotTolerance  = 8
	rgba16To8      = 8 // RGBA() returns 16-bit channels
)

// pageColor is #2e8b57, the /page background.
var pageColor = [3]int{0x2e, 0x8b, 0x57}

// shotCheck requires >= 20% of the screenshot to be the /page color within
// retries attempts. It returns the time until the first good shot.
func (r *stressRun) shotCheck(stage string, retries int) (time.Duration, bool) {
	start := time.Now()
	var ratio float64
	var lastErr error
	for i := range retries {
		if i > 0 {
			time.Sleep(shotRetryDelay)
		}
		if !r.proc.alive() {
			r.died = true
			r.failf("dumber died during %s screenshot check", stage)
			return 0, false
		}
		path, err := r.shot()
		if err == nil {
			ratio, err = colorRatio(path)
		}
		lastErr = err
		if err != nil {
			r.t.Logf("shot %s: %v", stage, err)
			continue
		}
		if ratio >= shotMinRatio {
			return time.Since(start), true
		}
	}
	if lastErr != nil {
		r.failf("%s screenshot: no decodable shot: %v", stage, lastErr)
	} else {
		r.failf("%s screenshot: page color covers %.1f%%, want >= %.0f%%", stage, ratio*100, shotMinRatio*100)
	}
	return 0, false
}

func (r *stressRun) shot() (string, error) {
	out, err := exec.Command("neferwl", "-s", r.sb.session, "shot").Output()
	if err != nil {
		return "", fmt.Errorf("neferwl shot: %w", err)
	}
	path, err := parseShotPath(out)
	if err != nil {
		return "", err
	}
	// Keep a copy in the sandbox: the shot lives in the NeferWL session dir,
	// which is removed before the failure artifacts are collected.
	keep := filepath.Join(r.sb.root, "last-shot.png")
	if err := copyFile(path, keep); err != nil {
		return "", fmt.Errorf("keep shot: %w", err)
	}
	r.lastShot = keep
	return keep, nil
}

func parseShotPath(out []byte) (string, error) {
	var res struct {
		Shots []struct {
			Path string `json:"path"`
		} `json:"shots"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return "", fmt.Errorf("parse shot json: %w", err)
	}
	if len(res.Shots) == 0 || res.Shots[0].Path == "" {
		return "", fmt.Errorf("shot json has no path: %s", out)
	}
	return res.Shots[0].Path, nil
}

func colorRatio(path string) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		return 0, fmt.Errorf("decode %s: %w", path, err)
	}
	if img.Bounds().Empty() {
		return 0, errors.New("empty shot " + path)
	}
	return matchRatio(img, pageColor, shotTolerance), nil
}

func matchRatio(img image.Image, want [3]int, tol int) float64 {
	b := img.Bounds()
	total, match := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			total++
			if near(int(cr>>rgba16To8), want[0], tol) && near(int(cg>>rgba16To8), want[1], tol) && near(int(cb>>rgba16To8), want[2], tol) {
				match++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(match) / float64(total)
}

func near(a, b, tol int) bool { return a-b <= tol && b-a <= tol }
