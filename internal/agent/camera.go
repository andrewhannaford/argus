package agent

import (
	"encoding/base64"
	"runtime"
	"strings"
	"time"
)

type Camera struct {
	onFrame func(string)
	stop    chan struct{}
}

func NewCamera(onFrame func(string)) *Camera {
	return &Camera{onFrame: onFrame, stop: make(chan struct{})}
}

func (c *Camera) Start() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			frame, err := captureFrame()
			if err != nil {
				continue
			}
			c.onFrame(frame)
		}
	}
}

func (c *Camera) Stop() {
	select {
	case <-c.stop:
	default:
		close(c.stop)
	}
}

func captureFrame() (string, error) {
	args := cameraArgs()
	cmd := newCommand("ffmpeg", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(out), nil
}

func cameraArgs() []string {
	base := []string{"-loglevel", "quiet", "-vframes", "1", "-f", "image2", "-vcodec", "mjpeg", "pipe:1"}
	switch runtime.GOOS {
	case "windows":
		device := windowsCameraDevice()
		return append([]string{"-f", "dshow", "-i", "video=" + device}, base...)
	case "linux":
		return append([]string{"-f", "v4l2", "-i", "/dev/video0"}, base...)
	default: // darwin
		return append([]string{"-f", "avfoundation", "-framerate", "30", "-i", "0"}, base...)
	}
}

// windowsCameraDevice returns the first DirectShow video device name.
func windowsCameraDevice() string {
	cmd := newCommand("ffmpeg", "-f", "dshow", "-list_devices", "true", "-i", "dummy", "-loglevel", "info")
	out, _ := cmd.CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "(video)") {
			s := strings.Index(line, "\"")
			e := strings.LastIndex(line, "\"")
			if s >= 0 && e > s {
				return line[s+1 : e]
			}
		}
	}
	return "default"
}
