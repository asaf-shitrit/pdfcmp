package render

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pdfSpec describes a one-page PDF with the usual font /F1, an image /Im
// (object 6, replaceable through Image) and extra objects numbered from 7.
type pdfSpec struct {
	Catalog string   // extra catalog entries
	Page    string   // extra page entries
	Res     string   // extra resource entries
	Content string   // content stream
	Image   string   // full object 6 (an image XObject), or "" for a 2x2 gray one
	Objs    []string // objects 7, 8, ...: the body between "obj" and "endobj"
	Pages   string   // boxes written on /Pages, inherited by the page; "" puts MediaBox [0 0 600 800] on the page
}

func (s pdfSpec) write(path string) error {
	var b bytes.Buffer
	var offs []int
	add := func(o string) { offs = append(offs, b.Len()); b.WriteString(o) }
	img := s.Image
	if img == "" {
		img = "<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace /DeviceGray /BitsPerComponent 8 /Length 4 >>\nstream\n\x00\xff\xff\xff\nendstream"
	}
	b.WriteString("%PDF-1.7\n")
	add(fmt.Sprintf("1 0 obj\n<< /Type /Catalog /Pages 2 0 R %s >>\nendobj\n", s.Catalog))
	media := "/MediaBox [0 0 600 800]"
	if s.Pages != "" {
		media = ""
	}
	add(fmt.Sprintf("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 %s >>\nendobj\n", s.Pages))
	add(fmt.Sprintf("3 0 obj\n<< /Type /Page /Parent 2 0 R %s %s /Resources << /Font << /F1 5 0 R >> /XObject << /Im 6 0 R >> %s >> /Contents 4 0 R >>\nendobj\n", media, s.Page, s.Res))
	add(fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(s.Content), s.Content))
	add("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")
	add("6 0 obj\n" + img + "\nendobj\n")
	for i, o := range s.Objs {
		add(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", 7+i, o))
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offs)+1, xref)
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// hashResult is a page's content hash, or the error ContentHash returned.
type hashResult struct {
	hash uint64
	err  error
}

func hashSpecs(t *testing.T, specs ...pdfSpec) []hashResult {
	t.Helper()
	renderer, err := NewPdfiumRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer renderer.Close()
	dir := t.TempDir()
	var out []hashResult
	for i, s := range specs {
		path := filepath.Join(dir, fmt.Sprintf("s%d.pdf", i))
		if err := s.write(path); err != nil {
			t.Fatal(err)
		}
		doc, err := renderer.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		h, err := doc.ContentHash(0)
		_ = doc.Close()
		out = append(out, hashResult{h, err})
	}
	return out
}

// trusted reports whether the planner may skip rendering for the two pages:
// both hashed, to the same value.
func trusted(r []hashResult) bool {
	return r[0].err == nil && r[1].err == nil && r[0].hash == r[1].hash
}

const (
	bText = "BT /F1 24 Tf 30 400 Td (Hello world invoice 12345) Tj ET "
	// nested squares wound the same way: nonzero fills both, even-odd leaves a hole.
	nested = "100 100 200 200 re 150 150 100 100 re "
)

// A page that draws or looks different in a way the hash cannot see must
// either hash differently or not be hashable (ErrNotHashable), never hash
// equal: the planner scores equal hashes 1.0 without rendering.
func TestContentHashDoesNotTrustWhatItCannotSee(t *testing.T) {
	var poly1, poly2 strings.Builder
	poly1.WriteString("0 0 m ")
	poly2.WriteString("0 0 m ")
	for i := 1; i < 600; i++ {
		fmt.Fprintf(&poly1, "%d %d l ", i, 100+i%50)
		y := 100 + i%50
		if i == 301 {
			y = 140
		}
		fmt.Fprintf(&poly2, "%d %d l ", i, y)
	}
	imgDict := func(extra string, cs string, body string) string {
		return fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace %s /BitsPerComponent 8 %s /Length %d >>\nstream\n%s\nendstream", cs, extra, len(body), body)
	}
	ocg := pdfSpec{Catalog: "/OCProperties << /OCGs [7 0 R] /D << /ON [7 0 R] >> >>", Res: "/Properties << /OC1 7 0 R >>", Objs: []string{"<< /Type /OCG /Name (DRAFT) >>"}}
	ocgOff := ocg
	ocgOff.Catalog = "/OCProperties << /OCGs [7 0 R] /D << /OFF [7 0 R] >> >>"
	shade := func(c string) pdfSpec {
		return pdfSpec{Res: "/Shading << /Sh1 << /ShadingType 2 /ColorSpace /DeviceRGB /Coords [0 0 600 0] /Function << /FunctionType 2 /Domain [0 1] /C0 [1 0 0] /C1 [" + c + "] /N 1 >> >> >>", Content: bText + "/Sh1 sh"}
	}
	ann := func(flags int) pdfSpec {
		return pdfSpec{
			Page:    "/Annots [7 0 R]",
			Content: bText,
			Objs: []string{
				fmt.Sprintf("<< /Type /Annot /Subtype /Square /Rect [100 100 300 300] /F %d /AP << /N 8 0 R >> >>", flags),
				"<< /Type /XObject /Subtype /Form /BBox [0 0 200 200] /Length 28 >>\nstream\n1 0 0 rg 0 0 200 200 re f\nendstream",
			},
		}
	}
	draw := func(c string) pdfSpec { return pdfSpec{Content: bText + c} }
	img := func(i string) pdfSpec { return pdfSpec{Content: bText + "q 300 0 0 300 100 50 cm /Im Do Q", Image: i} }

	for _, tc := range []struct {
		name string
		a, b pdfSpec
	}{
		{"layer on against off", func() pdfSpec {
			s := ocg
			s.Content = bText + "/OC /OC1 BDC 1 0 0 rg 100 100 300 300 re f EMC"
			return s
		}(), func() pdfSpec {
			s := ocgOff
			s.Content = bText + "/OC /OC1 BDC 1 0 0 rg 100 100 300 300 re f EMC"
			return s
		}()},
		{"stroke against fill and stroke", draw("0 0 1 RG 1 0 0 rg 8 w 100 100 200 200 re S"), draw("0 0 1 RG 1 0 0 rg 8 w 100 100 200 200 re B")},
		{"nonzero against even-odd fill", draw("0 0 1 rg " + nested + "f"), draw("0 0 1 rg " + nested + "f*")},
		{"line cap", draw("0 0 1 RG 20 w 0 J 100 100 m 300 100 l S"), draw("0 0 1 RG 20 w 1 J 100 100 m 300 100 l S")},
		{"line join", draw("0 0 1 RG 20 w 0 j 100 100 m 300 300 l 300 100 l S"), draw("0 0 1 RG 20 w 1 j 100 100 m 300 300 l 300 100 l S")},
		{"dash added", draw("0 0 1 RG 6 w 100 100 m 500 100 l S"), draw("0 0 1 RG 6 w [10 10] 0 d 100 100 m 500 100 l S")},
		{"dash phase", draw("0 0 1 RG 6 w [10 10] 0 d 100 100 m 500 100 l S"), draw("0 0 1 RG 6 w [10 10] 5 d 100 100 m 500 100 l S")},
		{"stroke width", draw("0 0 1 RG 6 w 100 100 m 500 100 l S"), draw("0 0 1 RG 12 w 100 100 m 500 100 l S")},
		{"one moved point in a 600-point chart", draw("0 0 1 rg " + poly1.String() + "f"), draw("0 0 1 rg " + poly2.String() + "f")},
		{"shading recoloured", shade("0 0 1"), shade("0 1 0")},
		{"image with /Decode", img(imgDict("", "/DeviceGray", "\x00\xff\xff\xff")), img(imgDict("/Decode [1 0]", "/DeviceGray", "\x00\xff\xff\xff"))},
		{"image palette against gray", img(imgDict("", "/DeviceGray", "\x00\x01\x01\x01")), img(imgDict("", "[/Indexed /DeviceRGB 1 <ff0000 0000ff>]", "\x00\x01\x01\x01"))},
		{"annotation print against hidden", ann(4), ann(2)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := hashSpecs(t, tc.a, tc.b)
			if trusted(r) {
				t.Errorf("hashes %x and %x are equal and trustworthy; want a different hash or ErrNotHashable", r[0].hash, r[1].hash)
			}
			for _, x := range r {
				if x.err != nil && !errors.Is(x.err, ErrNotHashable) {
					t.Errorf("ContentHash failed with %v, want a hash or ErrNotHashable", x.err)
				}
			}
		})
	}
}

// Pages the hash can vouch for still hash, and hash the same when they are
// the same: the new checks must not turn every stroked path or image into a
// render.
func TestContentHashStillTrustsPlainStrokesAndImages(t *testing.T) {
	stroke := pdfSpec{Content: bText + "0 0 1 RG 6 w [10 10] 0 d 1 J 100 100 m 500 100 l S 100 200 200 100 re B"}
	image := pdfSpec{Content: bText + "q 300 0 0 300 100 50 cm /Im Do Q"}
	for name, s := range map[string]pdfSpec{"stroke": stroke, "image": image} {
		r := hashSpecs(t, s, s)
		if !trusted(r) {
			t.Errorf("%s: identical pages are not trusted: %v %v", name, r[0].err, r[1].err)
		}
	}
}

// A page with more path segments than the budget is not hashable.
func TestContentHashBailsOverSegmentBudget(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(bText + "0 0 m ")
	for i := 0; i <= maxPageSegments; i++ {
		fmt.Fprintf(&sb, "%d %d l ", i%500, i%700)
	}
	sb.WriteString("S")
	r := hashSpecs(t, pdfSpec{Content: sb.String()})
	if !errors.Is(r[0].err, ErrNotHashable) {
		t.Errorf("ContentHash = %x, %v, want ErrNotHashable", r[0].hash, r[0].err)
	}
}
