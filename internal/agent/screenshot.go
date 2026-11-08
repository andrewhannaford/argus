package agent

import (
	"bytes"
	"encoding/base64"
	"image/png"

	"github.com/kbinani/screenshot"
)

func captureScreenshot(display int) (string, error) {
	if display < 0 || display >= screenshot.NumActiveDisplays() {
		display = 0
	}
	bounds := screenshot.GetDisplayBounds(display)
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
