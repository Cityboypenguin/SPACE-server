package upload

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/Cityboypenguin/SPACE-server/internal/apperr"
)

const maxStampSourceBytes = 20 * mb
const maxStampStoredBytes = 2 * mb
const maxStampSide = 1024

var svgRootAttributes = regexp.MustCompile(`\s+([A-Za-z_:][A-Za-z0-9_.:-]*)\s*=\s*(?:"[^"]*"|'[^']*')`)

var localSVGURL = regexp.MustCompile(`^url\(#[A-Za-z_][A-Za-z0-9_.:-]*\)$`)
var svgTags = strings.Fields("svg g defs path rect circle ellipse line polyline polygon use symbol clipPath mask linearGradient radialGradient stop title desc text tspan")
var svgAttrs = strings.Fields("id viewBox width height x y x1 y1 x2 y2 cx cy r rx ry d points transform fill fill-opacity fill-rule stroke stroke-width stroke-opacity stroke-linecap stroke-linejoin stroke-miterlimit stroke-dasharray stroke-dashoffset opacity clip-path clip-rule mask href preserveAspectRatio gradientUnits gradientTransform spreadMethod offset stop-color stop-opacity fx fy fr font-size font-family font-weight text-anchor dominant-baseline dx dy rotate textLength lengthAdjust color vector-effect xmlns version")

func includes(xs []string, x string) bool {
	for _, v := range xs {
		if x == v {
			return true
		}
	}
	return false
}

// ValidateStampImage validates the entire bounded file. SVG is an intentionally static subset.
func ValidateStampImage(contentType string, body []byte) error {
	bad := func() error {
		return apperr.InvalidInput("静止画のPNGまたは対応するSVGを指定してください（元画像は20MiB以下）")
	}
	if len(body) == 0 || len(body) > maxStampSourceBytes {
		return bad()
	}
	if contentType == "image/png" {
		cfg, e := png.DecodeConfig(bytes.NewReader(body))
		if e != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 32768 || cfg.Height > 32768 || int64(cfg.Width)*int64(cfg.Height) > 32*1024*1024 {
			return bad()
		}
		// PNG chunk parsing avoids mistaking arbitrary compressed bytes for APNG markers.
		for pos := 8; pos+12 <= len(body); {
			n := int(binary.BigEndian.Uint32(body[pos : pos+4]))
			if n > len(body)-pos-12 {
				return bad()
			}
			if string(body[pos+4:pos+8]) == "acTL" {
				return bad()
			}
			pos += 12 + n
		}
		if _, e = png.Decode(bytes.NewReader(body)); e != nil {
			return bad()
		}
		return nil
	}
	if contentType != "image/svg+xml" {
		return bad()
	}
	dec := xml.NewDecoder(bytes.NewReader(body))
	depth, nodes, roots := 0, 0, 0
	for {
		tok, e := dec.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return bad()
		}
		switch t := tok.(type) {
		case xml.Directive:
			return bad() // no DTD/entities
		case xml.ProcInst:
			if t.Target != "xml" || roots > 0 {
				return bad()
			}
		case xml.StartElement:
			depth++
			nodes++
			if depth > 64 || nodes > 10000 || len(t.Attr) > 64 {
				return bad()
			}
			if !includes(svgTags, t.Name.Local) || (t.Name.Space != "" && t.Name.Space != "http://www.w3.org/2000/svg") {
				return apperr.InvalidInput("SVGに未対応の要素があります。style・スクリプト・アニメーションを含めず、パス主体の静止画として書き出してください")
			}
			if depth == 1 {
				roots++
				if roots != 1 || t.Name.Local != "svg" {
					return bad()
				}
			}
			hasBounds := false
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
					continue
				}
				if a.Name.Space != "" && a.Name.Space != "http://www.w3.org/1999/xlink" {
					return bad()
				}
				if !includes(svgAttrs, a.Name.Local) {
					return bad()
				}
				value := strings.TrimSpace(a.Value)
				lower := strings.ToLower(value)
				if strings.ContainsAny(value, "\\") || strings.Contains(value, "/*") {
					return bad()
				}
				if strings.Contains(lower, "url") && !localSVGURL.MatchString(value) {
					return bad()
				}
				if a.Name.Local == "href" && (!strings.HasPrefix(value, "#") || len(value) < 2) {
					return bad()
				}
				if depth == 1 && a.Name.Local == "viewBox" {
					parts := strings.Fields(strings.ReplaceAll(value, ",", " "))
					if len(parts) != 4 {
						return bad()
					}
					for i, p := range parts {
						v, e := strconv.ParseFloat(p, 64)
						if e != nil || v != v || v > 1e6 || v < -1e6 {
							return bad()
						}
						if i >= 2 && (v <= 0) {
							return bad()
						}
					}
					hasBounds = true
				}
			}
			if depth == 1 && !hasBounds {
				return apperr.InvalidInput("SVGには幅・高さが正のviewBoxが必要です")
			}
		case xml.EndElement:
			depth--
		case xml.CharData:
			if depth == 0 && strings.TrimSpace(string(t)) != "" {
				return bad()
			}
		}
	}
	if roots != 1 || depth != 0 {
		return bad()
	}
	return nil
}

func normalizeStampPNG(body []byte) ([]byte, error) {
	if err := ValidateStampImage("image/png", body); err != nil {
		return nil, err
	}
	img, err := png.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	width, height := stampFitSize(img.Bounds().Dx(), img.Bounds().Dy(), maxStampSide)
	for {
		resized := img
		if width != img.Bounds().Dx() || height != img.Bounds().Dy() {
			resized = resizeStampImage(img, width, height)
		}
		var out bytes.Buffer
		if err := png.Encode(&out, resized); err != nil {
			return nil, err
		}
		if out.Len() <= maxStampStoredBytes {
			return out.Bytes(), nil
		}
		if width == 1 && height == 1 {
			return nil, apperr.InvalidInput("画像を2MiB以下に縮小できませんでした")
		}
		// Retain the source for each pass to avoid cumulative resampling loss.
		side := max(1, int(float64(max(width, height))*math.Sqrt(float64(maxStampStoredBytes)/float64(out.Len()))*0.9))
		width, height = stampFitSize(img.Bounds().Dx(), img.Bounds().Dy(), side)
	}
}

func stampFitSize(width, height, limit int) (int, int) {
	scale := math.Min(1, float64(limit)/float64(max(width, height)))
	return max(1, int(math.Round(float64(width)*scale))), max(1, int(math.Round(float64(height)*scale)))
}

// Area averaging preserves thin strokes and uses premultiplied alpha to avoid dark halos.
func resizeStampImage(src image.Image, width, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	bounds := src.Bounds()
	sx, sy := float64(bounds.Dx())/float64(width), float64(bounds.Dy())/float64(height)
	for y := 0; y < height; y++ {
		y0, y1 := float64(y)*sy, float64(y+1)*sy
		for x := 0; x < width; x++ {
			x0, x1 := float64(x)*sx, float64(x+1)*sx
			var r, g, b, a, weight float64
			for iy := int(y0); iy < int(math.Ceil(y1)); iy++ {
				wy := math.Min(y1, float64(iy+1)) - math.Max(y0, float64(iy))
				for ix := int(x0); ix < int(math.Ceil(x1)); ix++ {
					w := wy * (math.Min(x1, float64(ix+1)) - math.Max(x0, float64(ix)))
					cr, cg, cb, ca := src.At(bounds.Min.X+ix, bounds.Min.Y+iy).RGBA()
					r += float64(cr) * w
					g += float64(cg) * w
					b += float64(cb) * w
					a += float64(ca) * w
					weight += w
				}
			}
			dst.SetRGBA(x, y, color.RGBA{R: uint8(math.Round(r / weight / 257)), G: uint8(math.Round(g / weight / 257)), B: uint8(math.Round(b / weight / 257)), A: uint8(math.Round(a / weight / 257))})
		}
	}
	return dst
}

// Keep SVG coordinate geometry and references intact; only its intrinsic viewport shrinks.
func normalizeStampSVG(body []byte) ([]byte, error) {
	if err := ValidateStampImage("image/svg+xml", body); err != nil {
		return nil, err
	}
	dec := xml.NewDecoder(bytes.NewReader(body))
	for {
		token, err := dec.Token()
		if err != nil {
			return nil, err
		}
		root, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		var width, height float64
		for _, attr := range root.Attr {
			if attr.Name.Local == "viewBox" {
				parts := strings.Fields(strings.ReplaceAll(attr.Value, ",", " "))
				width, _ = strconv.ParseFloat(parts[2], 64)
				height, _ = strconv.ParseFloat(parts[3], 64)
			}
		}
		scale := math.Min(1, float64(maxStampSide)/math.Max(width, height))
		width *= scale
		height *= scale
		end := int(dec.InputOffset())
		start := bytes.LastIndexByte(body[:end], '<')
		tag := svgRootAttributes.ReplaceAllStringFunc(string(body[start:end]), func(attribute string) string {
			name := svgRootAttributes.FindStringSubmatch(attribute)[1]
			if name == "width" || name == "height" || name == "preserveAspectRatio" {
				return ""
			}
			return attribute
		})
		closing := ">"
		if strings.HasSuffix(tag, "/>") {
			closing = "/>"
		}
		tag = strings.TrimSuffix(tag, closing) + ` width="` + strconv.FormatFloat(width, 'f', -1, 64) + `" height="` + strconv.FormatFloat(height, 'f', -1, 64) + `" preserveAspectRatio="xMidYMid meet"` + closing
		result := make([]byte, 0, len(body)+100)
		result = append(result, body[:start]...)
		result = append(result, tag...)
		result = append(result, body[end:]...)
		if len(result) > maxStampStoredBytes {
			return nil, apperr.InvalidInput("SVGのファイルサイズは2MiB以下にしてください")
		}
		return result, nil
	}
}
