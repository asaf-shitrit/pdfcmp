package fixtures

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/asaf-shitrit/pdfcmp/pkg/render"
)

func TestGenerate(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(dir, io.Discard); err != nil {
		t.Fatal(err)
	}

	r, err := render.NewPdfiumRenderer()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	texts := map[string][]uint64{}
	for _, s := range Set {
		doc, err := r.Open(dir + "/" + s.Name)
		if err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
		if got := doc.PageCount(); got != s.Pages {
			t.Errorf("%s: %d pages, want %d", s.Name, got, s.Pages)
		}
		for p := 0; p < doc.PageCount(); p++ {
			info, err := doc.PageInfo(p)
			if err != nil {
				t.Fatalf("%s page %d: %v", s.Name, p+1, err)
			}
			texts[s.Name] = append(texts[s.Name], info.TextHash)
		}
		doc.Close()
	}

	// Pairs meant to be identical hold the same text on every page.
	for _, pair := range [][2]string{{"small_a.pdf", "small_b.pdf"}, {"medium_a.pdf", "medium_b.pdf"}, {"large_a.pdf", "large_b.pdf"}, {"mixed_a.pdf", "mixed_b.pdf"}} {
		a, b := texts[pair[0]], texts[pair[1]]
		for p := range a {
			if a[p] != b[p] {
				t.Errorf("%s and %s differ on page %d", pair[0], pair[1], p+1)
			}
		}
	}

	// Each *_changed file differs from its original on every page.
	for _, pair := range [][2]string{{"small_a.pdf", "small_changed.pdf"}, {"medium_a.pdf", "medium_changed.pdf"}, {"large_a.pdf", "large_changed.pdf"}, {"xlarge.pdf", "xlarge_changed.pdf"}} {
		a, c := texts[pair[0]], texts[pair[1]]
		for p := range a {
			if a[p] == c[p] {
				t.Errorf("%s and %s have the same text on page %d", pair[0], pair[1], p+1)
			}
		}
	}

	// medium_modified differs from medium_a on page 10 only.
	a, m := texts["medium_a.pdf"], texts["medium_modified.pdf"]
	for p := range a {
		if differs := a[p] != m[p]; differs != (p+1 == 10) {
			t.Errorf("medium_modified page %d: differs = %v, want %v", p+1, differs, p+1 == 10)
		}
	}
}

// Same content, different names: the bytes must differ, or a comparison of
// the pair would stop at the byte check without rendering.
func TestSameContentPairsDifferInBytes(t *testing.T) {
	dir := t.TempDir()
	read := func(name string) []byte {
		s, _ := Find(name)
		path, err := s.Write(dir)
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, pair := range [][2]string{{"small_a.pdf", "small_b.pdf"}, {"graphic_a.pdf", "graphic_b.pdf"}, {"scanned_a.pdf", "scanned_b.pdf"}, {"mixed_a.pdf", "mixed_b.pdf"}} {
		if bytes.Equal(read(pair[0]), read(pair[1])) {
			t.Errorf("%s and %s are byte-identical", pair[0], pair[1])
		}
	}
}

func TestFind(t *testing.T) {
	if s, ok := Find("medium_modified.pdf"); !ok || s.Pages != 20 {
		t.Errorf("Find(medium_modified.pdf) = %+v, %v", s, ok)
	}
	if _, ok := Find("missing.pdf"); ok {
		t.Error("Find(missing.pdf) found a fixture")
	}
}

func TestWriteReportsErrors(t *testing.T) {
	s, _ := Find("single_a.pdf")
	if _, err := s.Write(t.TempDir() + "/no/such/dir"); err == nil {
		t.Error("want an error writing into a missing directory")
	}
}
