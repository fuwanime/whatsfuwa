package app

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"strings"
)

type ImageProtocol string

const (
	ImageAuto  ImageProtocol = "auto"
	ImageKitty ImageProtocol = "kitty"
	ImageSixel ImageProtocol = "sixel"
	ImageNone  ImageProtocol = "none"
)

func DetectImageProtocol(getenv func(string) string) ImageProtocol {
	switch strings.ToLower(getenv("WHATSFUWA_IMAGE_PROTOCOL")) {
	case "kitty":
		return ImageKitty
	case "sixel":
		return ImageSixel
	case "none":
		return ImageNone
	}
	if getenv("KITTY_WINDOW_ID") != "" || getenv("GHOSTTY_RESOURCES_DIR") != "" || getenv("WEZTERM_EXECUTABLE") != "" || getenv("KONSOLE_VERSION") != "" {
		return ImageKitty
	}
	term := strings.ToLower(getenv("TERM"))
	if strings.Contains(term, "kitty") || strings.Contains(term, "ghostty") {
		return ImageKitty
	}
	if strings.Contains(term, "sixel") || term == "mlterm" || term == "foot" || term == "contour" {
		return ImageSixel
	}
	return ImageNone
}

func WriteImagePreview(output io.Writer, path string, protocol ImageProtocol, columns, rows int) error {
	if protocol != ImageKitty && protocol != ImageSixel {
		return fmt.Errorf("terminal tidak mendukung preview gambar; atur WHATSFUWA_IMAGE_PROTOCOL=kitty atau sixel")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		return fmt.Errorf("membuka gambar: %w", err)
	}
	if columns < 1 {
		columns = 80
	}
	if rows < 1 {
		rows = 24
	}
	img = fitImage(img, columns, rows)
	switch protocol {
	case ImageKitty:
		width := (img.Bounds().Dx() + 7) / 8
		height := (img.Bounds().Dy() + 15) / 16
		encoded, err := encodeKitty(img, minInt(columns, width), minInt(rows, height))
		if err != nil {
			return err
		}
		_, err = io.WriteString(output, encoded)
		return err
	case ImageSixel:
		_, err := io.WriteString(output, encodeSixel(img))
		return err
	}
	return nil
}

func fitImage(source image.Image, columns, rows int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	maxWidth, maxHeight := columns*8, rows*16
	newWidth, newHeight := width, height
	if newWidth > maxWidth {
		newHeight = maxInt(1, newHeight*maxWidth/newWidth)
		newWidth = maxWidth
	}
	if newHeight > maxHeight {
		newWidth = maxInt(1, newWidth*maxHeight/newHeight)
		newHeight = maxHeight
	}
	if newWidth < 1 {
		newWidth = 1
	}
	if newHeight < 1 {
		newHeight = 1
	}
	result := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	for y := 0; y < newHeight; y++ {
		sourceY := bounds.Min.Y + y*height/newHeight
		for x := 0; x < newWidth; x++ {
			sourceX := bounds.Min.X + x*width/newWidth
			result.Set(x, y, source.At(sourceX, sourceY))
		}
	}
	return result
}

func encodeKitty(img image.Image, columns, rows int) (string, error) {
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, img); err != nil {
		return "", err
	}
	encoded := base64.StdEncoding.EncodeToString(pngData.Bytes())
	const chunkSize = 4096
	var output strings.Builder
	for offset := 0; offset < len(encoded); offset += chunkSize {
		end := minInt(len(encoded), offset+chunkSize)
		last := end == len(encoded)
		output.WriteString("\x1b_G")
		if offset == 0 {
			fmt.Fprintf(&output, "a=T,f=100,t=d,q=2,c=%d,r=%d,", columns, rows)
		} else {
			output.WriteString("q=2,")
		}
		if last {
			output.WriteString("m=0;")
		} else {
			output.WriteString("m=1;")
		}
		output.WriteString(encoded[offset:end])
		output.WriteString("\x1b\\")
	}
	return output.String(), nil
}

func encodeSixel(img image.Image) string {
	bounds := img.Bounds()
	used := make([]bool, 217)
	indices := make([]int, bounds.Dx()*bounds.Dy())
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			index := sixelColorIndex(img.At(bounds.Min.X+x, bounds.Min.Y+y))
			indices[y*bounds.Dx()+x] = index
			used[index] = true
		}
	}
	var output strings.Builder
	output.WriteString("\x1bPq")
	for index := 0; index < len(used); index++ {
		if used[index] {
			r, g, b := sixelPaletteRGB(index)
			fmt.Fprintf(&output, "#%d;2;%d;%d;%d", index, r, g, b)
		}
	}
	for top := 0; top < bounds.Dy(); top += 6 {
		for colorIndex := 0; colorIndex < len(used); colorIndex++ {
			if !used[colorIndex] {
				continue
			}
			fmt.Fprintf(&output, "#%d", colorIndex)
			for x := 0; x < bounds.Dx(); {
				value := byte(0)
				for bit := 0; bit < 6; bit++ {
					y := top + bit
					if y < bounds.Dy() && indices[y*bounds.Dx()+x] == colorIndex {
						value |= 1 << bit
					}
				}
				sixel := byte(63) + value
				end := x + 1
				for end < bounds.Dx() {
					next := byte(0)
					for bit := 0; bit < 6; bit++ {
						y := top + bit
						if y < bounds.Dy() && indices[y*bounds.Dx()+end] == colorIndex {
							next |= 1 << bit
						}
					}
					if byte(63)+next != sixel {
						break
					}
					end++
				}
				writeSixelRun(&output, sixel, end-x)
				x = end
			}
			output.WriteByte('$')
		}
		output.WriteByte('-')
	}
	output.WriteString("\x1b\\")
	return output.String()
}

func sixelColorIndex(value color.Color) int {
	r, g, b, a := value.RGBA()
	if a < 0x8000 {
		return 0
	}
	red := int((r>>8)*5+127) / 255
	green := int((g>>8)*5+127) / 255
	blue := int((b>>8)*5+127) / 255
	return 1 + 36*red + 6*green + blue
}

func sixelPaletteRGB(index int) (int, int, int) {
	if index == 0 {
		return 0, 0, 0
	}
	index--
	return (index / 36) * 20, ((index / 6) % 6) * 20, (index % 6) * 20
}

func writeSixelRun(output *strings.Builder, value byte, count int) {
	if count >= 3 {
		fmt.Fprintf(output, "!%d%c", count, value)
		return
	}
	for i := 0; i < count; i++ {
		output.WriteByte(value)
	}
}
