package app

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func environment(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestDetectImageProtocol(t *testing.T) {
	for _, test := range []struct {
		name string
		env  map[string]string
		want ImageProtocol
	}{
		{"kitty", map[string]string{"KITTY_WINDOW_ID": "1"}, ImageKitty},
		{"ghostty", map[string]string{"GHOSTTY_RESOURCES_DIR": "/tmp/ghostty"}, ImageKitty},
		{"konsole", map[string]string{"KONSOLE_VERSION": "240800"}, ImageKitty},
		{"sixel term", map[string]string{"TERM": "xterm-sixel"}, ImageSixel},
		{"mlterm", map[string]string{"TERM": "mlterm"}, ImageSixel},
		{"unknown fallback", map[string]string{"TERM": "xterm"}, ImageNone},
		{"explicit sixel", map[string]string{"KITTY_WINDOW_ID": "1", "WHATSFUWA_IMAGE_PROTOCOL": "sixel"}, ImageSixel},
		{"disabled", map[string]string{"KITTY_WINDOW_ID": "1", "WHATSFUWA_IMAGE_PROTOCOL": "none"}, ImageNone},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := DetectImageProtocol(environment(test.env)); got != test.want {
				t.Fatalf("protocol = %v, want %v", got, test.want)
			}
		})
	}
}

func TestKittyProtocolContainsPNG(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	output, err := encodeKitty(img, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(output, "\x1b_Ga=T,f=100") || !strings.Contains(output, "q=2") || !strings.HasSuffix(output, "\x1b\\") {
		t.Fatalf("invalid kitty envelope: %q", output)
	}
	start := strings.Index(output, ";")
	end := strings.LastIndex(output, "\x1b\\")
	if start < 0 || end <= start {
		t.Fatalf("kitty payload missing: %q", output)
	}
	data, err := base64.StdEncoding.DecodeString(output[start+1 : end])
	if err != nil {
		t.Fatal(err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width != 2 || config.Height != 1 {
		t.Fatalf("kitty payload is not source PNG: %#v, %v", config, err)
	}
}

func TestSixelProtocolContainsPaletteAndRaster(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	output := encodeSixel(img)
	for _, expected := range []string{"\x1bPq", ";2;100;0;0", "@"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("sixel output missing %q: %q", expected, output)
		}
	}
	if !strings.HasSuffix(output, "\x1b\\") {
		t.Fatalf("sixel string terminator missing: %q", output)
	}
}

func TestFitImagePreservesAspectRatioAndBounds(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	fitted := fitImage(img, 2, 2)
	if got := fitted.Bounds().Size(); got.X != 16 || got.Y != 16 {
		t.Fatalf("fitted size = %v, want 16x16", got)
	}
}
