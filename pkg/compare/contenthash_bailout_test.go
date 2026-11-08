package compare

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// bailPDF writes a one-page 600x800 PDF with font /F1, the page entries,
// catalog entries, resources and content given, and extra objects numbered
// from 6 (no image).
func bailPDF(t *testing.T, path, catalog, pageExtra, res, content string, objs ...string) {
	t.Helper()
	var b bytes.Buffer
	var offs []int
	add := func(s string) { offs = append(offs, b.Len()); b.WriteString(s) }
	b.WriteString("%PDF-1.7\n")
	add(fmt.Sprintf("1 0 obj\n<< /Type /Catalog /Pages 2 0 R %s >>\nendobj\n", catalog))
	add("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")
	add(fmt.Sprintf("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 800] %s /Resources << /Font << /F1 5 0 R >> %s >> /Contents 4 0 R >>\nendobj\n", pageExtra, res))
	add(fmt.Sprintf("4 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", len(content), content))
	add("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")
	for i, o := range objs {
		add(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", 6+i, o))
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offs)+1, xref)
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A page the content hash cannot vouch for is rendered, not trusted: these
// pairs have the same text and object count, and hashed equal before the
// hash covered how paths are painted or bailed out on layers, shadings and
// images with a /Decode array or a palette. Each must be DIFFERENT.
func TestUnhashableOrRestyledPagesAreRenderedAndDifferent(t *testing.T) {
	const text = "BT /F1 24 Tf 30 400 Td (Hello world invoice 12345) Tj ET "
	const nested = "100 100 400 400 re 200 200 200 200 re "
	img := func(extra, cs string) string {
		return fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width 2 /Height 2 /ColorSpace %s /BitsPerComponent 8 %s /Length 4 >>\nstream\n\x00\xff\xff\x00\nendstream", cs, extra)
	}
	imgRes := "/XObject << /Im 6 0 R >>"
	imgDo := text + "q 500 0 0 500 50 50 cm /Im Do Q"
	ocContent := text + "/OC /OC1 BDC 0 0 0 rg 0 0 600 800 re f EMC"
	shading := func(c string) (string, string) {
		return "/Shading << /Sh1 << /ShadingType 2 /ColorSpace /DeviceRGB /Coords [0 0 600 0] /Function << /FunctionType 2 /Domain [0 1] /C0 [1 1 1] /C1 [" + c + "] /N 1 >> >> >>", text + "/Sh1 sh"
	}
	shRes, shA := shading("0 0 0")
	shResB, shBody := shading("1 1 1")
	annot := func(f int) []string {
		return []string{
			fmt.Sprintf("<< /Type /Annot /Subtype /Square /Rect [0 0 600 800] /F %d /AP << /N 7 0 R >> >>", f),
			"<< /Type /XObject /Subtype /Form /BBox [0 0 600 800] /Length 32 >>\nstream\n0 0 0 rg 0 0 600 800 re f\nendstream",
		}
	}
	type page struct {
		catalog, extra, res, body string
		objs                      []string
	}
	ocg := func(state string) page {
		return page{catalog: "/OCProperties << /OCGs [6 0 R] /D << /" + state + " [6 0 R] >> >>", res: "/Properties << /OC1 6 0 R >>", body: ocContent, objs: []string{"<< /Type /OCG /Name (DRAFT) >>"}}
	}
	for _, tc := range []struct {
		name string
		a, b page
	}{
		{"layer on against off", ocg("ON"), ocg("OFF")},
		{"stroke against fill and stroke", page{body: text + "0 0 0 RG 0 0 0 rg 40 w 100 100 400 400 re S"}, page{body: text + "0 0 0 RG 0 0 0 rg 40 w 100 100 400 400 re B"}},
		{"nonzero against even-odd fill", page{body: text + "0 0 0 rg " + nested + "f"}, page{body: text + "0 0 0 rg " + nested + "f*"}},
		{"shading recoloured", page{res: shRes, body: shA}, page{res: shResB, body: shBody}},
		{"image /Decode inverted", page{res: imgRes, body: imgDo, objs: []string{img("", "/DeviceGray")}}, page{res: imgRes, body: imgDo, objs: []string{img("/Decode [1 0]", "/DeviceGray")}}},
		{"image gray against palette", page{res: imgRes, body: imgDo, objs: []string{img("", "/DeviceGray")}}, page{res: imgRes, body: imgDo, objs: []string{img("", "[/Indexed /DeviceRGB 255 <"+palette()+">]")}}},
		{"annotation printed against hidden", page{extra: "/Annots [6 0 R]", body: text, objs: annot(4)}, page{extra: "/Annots [6 0 R]", body: text, objs: annot(2)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			a, b := filepath.Join(dir, "a.pdf"), filepath.Join(dir, "b.pdf")
			bailPDF(t, a, tc.a.catalog, tc.a.extra, tc.a.res, tc.a.body, tc.a.objs...)
			bailPDF(t, b, tc.b.catalog, tc.b.extra, tc.b.res, tc.b.body, tc.b.objs...)
			c, err := New()
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			r, err := c.Compare(context.Background(), a, b)
			if err != nil {
				t.Fatal(err)
			}
			if r.IsSimilar(DefaultSimilarityThreshold) || r.SimilarityScore == 1 {
				t.Errorf("scored %v, want DIFFERENT: %s", r.SimilarityScore, r.Summary())
			}
		})
	}
}

// palette is a 256-entry RGB palette in hex whose entry 0 is white and
// entry 255 is black: the inverse of the gray ramp.
func palette() string {
	var s []byte
	for i := 0; i < 256; i++ {
		s = append(s, fmt.Sprintf("%02x%02x%02x", 255-i, 255-i, 255-i)...)
	}
	return string(s)
}
