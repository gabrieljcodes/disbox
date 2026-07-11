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

// GenerateOGImage generates an OG image matching the old HTML template design.
func GenerateOGImage(name, size, hash, itemType string) ([]byte, error) {
	const width = 1200
	const height = 630

	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	runes := []rune(name)
	if len(runes) > 65 {
		name = string(runes[:62]) + "..."
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
		// Draw logo image scaled to 80x80 roughly
		// A simple way is to use DrawImage
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

	// Right Panel
	faceBold56 := truetype.NewFace(fontBold, &truetype.Options{Size: 56})
	dc.SetFontFace(faceBold56)
	dc.SetColor(parseHexColor("#0F3320"))
	dc.DrawStringWrapped(name, 420, 200, 0, 0, 700, 1.2, gg.AlignLeft)

	dc.SetColor(color.NRGBA{15, 80, 40, 76})
	dc.DrawRoundedRectangle(420, 350, 45, 3, 1.5)
	dc.Fill()

	faceBold18 := truetype.NewFace(fontBold, &truetype.Options{Size: 18})
	faceReg18 := truetype.NewFace(fontRegular, &truetype.Options{Size: 18})

	startY := 400
	drawMeta := func(label, value string, y int) {
		dc.SetFontFace(faceBold18)
		dc.SetColor(parseHexColor("#0D3320"))
		dc.DrawString(label, 420, float64(y))

		w, _ := dc.MeasureString(label)
		dc.SetFontFace(faceReg18)
		dc.SetColor(parseHexColor("#1F5535"))
		dc.DrawString(value, 420+w+8, float64(y))
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
