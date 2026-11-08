package agent

import (
	"encoding/base64"
	"runtime"
	"strconv"
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

func (c *Camera) Start(device string, quality int) {
	winDevice := ""
	if runtime.GOOS == "windows" && device == "" {
		winDevice = windowsCameraDevice()
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			frame, err := captureFrame(winDevice, device, quality)
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

func captureFrame(winDevice, device string, quality int) (string, error) {
	cmd := newCommand("ffmpeg", cameraArgs(winDevice, device, quality)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(out), nil
}

func cameraArgs(winDevice, device string, quality int) []string {
	base := []string{"-loglevel", "quiet", "-vframes", "1", "-f", "image2", "-vcodec", "mjpeg"}
	if quality > 0 {
		base = append(base, "-q:v", strconv.Itoa(quality))
	}
	base = append(base, "pipe:1")
	switch runtime.GOOS {
	case "windows":
		d := device
		if d == "" {
			d = winDevice
		}
		return append([]string{"-f", "dshow", "-i", "video=" + d}, base...)
	case "linux":
		d := device
		if d == "" {
			d = "/dev/video0"
		}
		return append([]string{"-f", "v4l2", "-i", d}, base...)
	default: // darwin
		d := device
		if d == "" {
			d = "0"
		}
		return append([]string{"-f", "avfoundation", "-framerate", "30", "-i", d}, base...)
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
