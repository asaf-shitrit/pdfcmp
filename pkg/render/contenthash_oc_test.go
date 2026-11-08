package render

import (
	"errors"
	"testing"
)

// A Form XObject with its own /OC entry is a layer that go-pdfium reports
// as no mark, so a document with layers must not be hashable.
func TestContentHashBailsOutForDocumentWithLayers(t *testing.T) {
	form := "<< /Type /XObject /Subtype /Form /BBox [0 0 600 800] /OC 7 0 R /Length 30 >>\nstream\n1 0 0 rg 100 100 300 300 re f\nendstream"
	spec := func(catalog string) pdfSpec {
		return pdfSpec{Catalog: catalog, Content: bText + "/Im Do", Image: form, Objs: []string{"<< /Type /OCG /Name (DRAFT) >>"}}
	}
	r := hashSpecs(t,
		spec("/OCProperties << /OCGs [7 0 R] /D << /ON [7 0 R] >> >>"),
		spec("/OCProperties << /OCGs [7 0 R] /D << /OFF [7 0 R] >> >>"),
		// The key written with a name escape is not in the file as text, and
		// the file mentions /ObjStm, so only a scan of PDFium's own copy sees it.
		spec("/OC#50roperties << /OCGs [7 0 R] /D << /OFF [7 0 R] >> >> /Hint /ObjStm"),
		pdfSpec{Content: bText + "0 0 1 rg 30 100 200 100 re f"},
	)
	for i := 0; i < 3; i++ {
		if !errors.Is(r[i].err, ErrNotHashable) {
			t.Errorf("document %d: err = %v, want ErrNotHashable", i, r[i].err)
		}
	}
	if r[3].err != nil {
		t.Errorf("document without layers: err = %v", r[3].err)
	}
}

// A box inherited from the /Pages node decides what is shown, so it is part
// of the page's hash.
func TestContentHashCoversInheritedBoxes(t *testing.T) {
	spec := func(boxes string) pdfSpec {
		return pdfSpec{Pages: boxes, Content: bText + "0 0 1 rg 30 100 200 100 re f"}
	}
	r := hashSpecs(t,
		spec("/MediaBox [0 0 600 800]"),
		spec("/MediaBox [0 0 600 800] /CropBox [0 0 300 500]"),
		spec("/MediaBox [0 0 1200 1600]"),
		spec("/MediaBox [0 0 600 800] /CropBox [0 0 600 800]"),
	)
	for _, x := range r {
		if x.err != nil {
			t.Fatal(x.err)
		}
	}
	if r[0].hash == r[1].hash || r[0].hash == r[2].hash || r[1].hash == r[2].hash {
		t.Errorf("inherited boxes hash alike: %x %x %x", r[0].hash, r[1].hash, r[2].hash)
	}
	if r[0].hash != r[3].hash {
		t.Errorf("inherited CropBox equal to MediaBox hashes unlike none: %x %x", r[0].hash, r[3].hash)
	}
}

func TestOCScannerFindsKeyAcrossWrites(t *testing.T) {
	var s ocScanner
	_, _ = s.Write([]byte("xx /OCProp"))
	_, _ = s.Write([]byte("erties yy"))
	if !s.found["OCProperties"] {
		t.Error("key split across two writes not found")
	}
}
