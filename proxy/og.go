package proxy

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"log"
	"path"
	"strings"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

var (
	fontRegular *truetype.Font
	fontBold    *truetype.Font
	logoImage   image.Image
)

func init() {
	var err error
	fontRegular, err = truetype.Parse(goregular.TTF)
	if err != nil {
		log.Fatalf("failed to parse GoRegular font: %v", err)
	}
	fontBold, err = truetype.Parse(gobold.TTF)
	if err != nil {
		log.Fatalf("failed to parse GoBold font: %v", err)
	}

	logoImage, _, err = image.Decode(bytes.NewReader(iconTransparentBytes))
	if err != nil {
		log.Printf("failed to decode icon transparent bytes: %v", err)
	}
}

// splitTitleWords splits a filename into chunks by spaces, underscores, hyphens, and dots.
func splitTitleWords(s string) []string {
	var words []string
	var cur strings.Builder
	for _, r := range s {
		cur.WriteRune(r)
		if r == ' ' || r == '_' || r == '-' || r == '.' {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	return words
}

// wrapTitleToLines formats title text to fit within maxWidth across at most maxLines.
func wrapTitleToLines(dc *gg.Context, name string, maxWidth float64, maxLines int) []string {
	words := splitTitleWords(name)
	if len(words) == 0 {
		return []string{name}
	}

	var lines []string
	var curLine string

	for _, w := range words {
		testLine := curLine + w
		width, _ := dc.MeasureString(strings.TrimSpace(testLine))
		if width <= maxWidth {
			curLine = testLine
		} else {
			if curLine != "" {
				lines = append(lines, strings.TrimSpace(curLine))
				curLine = w
			} else {
				lines = append(lines, strings.TrimSpace(w))
				curLine = ""
			}
		}
	}
	if curLine != "" {
		lines = append(lines, strings.TrimSpace(curLine))
	}

	if len(lines) > maxLines {
		lastLine := lines[maxLines-1]
		for len(lastLine) > 0 {
			w, _ := dc.MeasureString(lastLine + "...")
			if w <= maxWidth {
				lines[maxLines-1] = lastLine + "..."
				break
			}
			r := []rune(lastLine)
			lastLine = string(r[:len(r)-1])
		}
		lines = lines[:maxLines]
	}

	return lines
}

// fitTitle finds the best font size and line split that fits within maxWidth and maxHeight.
func fitTitle(dc *gg.Context, name string, maxWidth, maxHeight float64) (font.Face, []string, float64) {
	for size := 48.0; size >= 24.0; size -= 2.0 {
		face := truetype.NewFace(fontBold, &truetype.Options{Size: size})
		dc.SetFontFace(face)

		// 1. Try single line without splitting
		w, h := dc.MeasureString(name)
		if w <= maxWidth && h <= maxHeight {
			return face, []string{name}, size
		}

		// 2. Try wrapping into up to 2 lines
		lines := wrapTitleToLines(dc, name, maxWidth, 2)
		allFit := true
		for _, l := range lines {
			lw, _ := dc.MeasureString(l)
			if lw > maxWidth {
				allFit = false
				break
			}
		}
		totalH := float64(len(lines)) * (size * 1.22)
		if allFit && totalH <= maxHeight && len(lines) <= 2 {
			return face, lines, size
		}
	}

	// Fallback at size 24: wrap and truncate to fit 2 lines
	size := 24.0
	face := truetype.NewFace(fontBold, &truetype.Options{Size: size})
	dc.SetFontFace(face)
	lines := wrapTitleToLines(dc, name, maxWidth, 2)
	for i, l := range lines {
		w, _ := dc.MeasureString(l)
		if w > maxWidth {
			for len(l) > 0 {
				tw, _ := dc.MeasureString(l + "...")
				if tw <= maxWidth {
					lines[i] = l + "..."
					break
				}
				r := []rune(l)
				l = string(r[:len(r)-1])
			}
		}
	}
	return face, lines, size
}

// GenerateOGImage generates an OG image matching the old HTML template design.
func GenerateOGImage(name, size, hash, itemType string) ([]byte, error) {
	const width = 1200
	const height = 630

	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	if strings.TrimSpace(name) == "" {
		name = "Unknown File"
	}

	dc := gg.NewContext(width, height)

	// Background gradient
	grad := gg.NewLinearGradient(0, 0, width, height)
	grad.AddColorStop(0.0, parseHexColor("#5ECBA5"))
	grad.AddColorStop(0.55, parseHexColor("#9ADEA8"))
	grad.AddColorStop(1.0, parseHexColor("#C8EF9A"))
	dc.SetFillStyle(grad)
	dc.DrawRectangle(0, 0, width, height)
	dc.Fill()

	// Left Accent
	dc.SetColor(parseHexColor("#1EBF6A"))
	dc.DrawRoundedRectangle(0, 265, 8, 100, 4)
	dc.Fill()

	// Top Right Deco
	dc.SetColor(parseHexColor("#18C965"))
	dc.DrawRoundedRectangle(1200-60-80, 40, 80, 4, 2)
	dc.Fill()

	// Bottom Deco
	dc.DrawRoundedRectangle(1200-300-80, 630-45-4, 80, 4, 2)
	dc.Fill()

	// Divider
	divGrad := gg.NewLinearGradient(360, 0, 360, height)
	divGrad.AddColorStop(0.0, color.NRGBA{20, 100, 55, 0})
	divGrad.AddColorStop(0.20, color.NRGBA{20, 100, 55, 64})
	divGrad.AddColorStop(0.50, color.NRGBA{20, 100, 55, 76})
	divGrad.AddColorStop(0.80, color.NRGBA{20, 100, 55, 64})
	divGrad.AddColorStop(1.0, color.NRGBA{20, 100, 55, 0})
	dc.SetFillStyle(divGrad)
	dc.DrawRectangle(360, 0, 2, height)
	dc.Fill()

	// Left Panel
	if logoImage != nil {
		dc.DrawImage(logoImage, 50, 190)
	}

	faceBold42 := truetype.NewFace(fontBold, &truetype.Options{Size: 42})
	dc.SetFontFace(faceBold42)
	dc.SetColor(parseHexColor("#0F3320"))
	dc.DrawString("Disbox", 50, 340)

	faceBold14 := truetype.NewFace(fontBold, &truetype.Options{Size: 14})
	dc.SetFontFace(faceBold14)
	dc.SetColor(parseHexColor("#2D6B44"))
	dc.DrawString("FILE SHARING", 50, 365)

	// Right Panel Title (Dynamic size & Wrapping)
	maxWidth := 700.0
	maxTitleH := 160.0
	titleFace, titleLines, fontSize := fitTitle(dc, name, maxWidth, maxTitleH)

	dc.SetFontFace(titleFace)
	dc.SetColor(parseHexColor("#0F3320"))

	lineSpacing := fontSize * 1.25
	titleStartY := 210.0
	if len(titleLines) > 1 {
		titleStartY = 180.0
	}

	for i, line := range titleLines {
		dc.DrawString(line, 420, titleStartY+float64(i)*lineSpacing)
	}

	// Accent Bar below title
	accentY := titleStartY + float64(len(titleLines)-1)*lineSpacing + 35
	dc.SetColor(color.NRGBA{15, 80, 40, 76})
	dc.DrawRoundedRectangle(420, accentY, 45, 3, 1.5)
	dc.Fill()

	// Metadata Labels & Values
	faceBold18 := truetype.NewFace(fontBold, &truetype.Options{Size: 18})
	faceReg18 := truetype.NewFace(fontRegular, &truetype.Options{Size: 18})

	startY := accentY + 40
	drawMeta := func(label, value string, y float64) {
		dc.SetFontFace(faceBold18)
		dc.SetColor(parseHexColor("#0D3320"))
		dc.DrawString(label, 420, y)

		w, _ := dc.MeasureString(label)
		dc.SetFontFace(faceReg18)
		dc.SetColor(parseHexColor("#1F5535"))
		dc.DrawString(value, 420+w+8, y)
	}

	drawMeta("TYPE:", itemType, startY)
	drawMeta("SIZE:", size, startY+30)
	drawMeta("HASH:", hash, startY+60)

	buf := new(bytes.Buffer)
	if err := dc.EncodePNG(buf); err != nil {
		return nil, fmt.Errorf("failed to encode png: %w", err)
	}

	return buf.Bytes(), nil
}

func parseHexColor(s string) color.Color {
	c := color.NRGBA{A: 255}
	if len(s) > 0 && s[0] == '#' {
		s = s[1:]
	}
	if len(s) == 6 {
		fmt.Sscanf(s, "%02x%02x%02x", &c.R, &c.G, &c.B)
	}
	return c
}
