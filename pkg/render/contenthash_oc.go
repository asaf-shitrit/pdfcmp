package render

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"

	"github.com/klippa-app/go-pdfium/requests"
)

// hasOptionalContent reports whether the document defines optional content
// groups (layers): its catalog has /OCProperties. The answer is computed
// once per document. A Form or Image XObject with its own /OC entry is
// hidden or shown by a layer's default state, but go-pdfium reports it as
// no mark on the page object, so the hash cannot see it; a document with
// layers is not hashable at all. go-pdfium has no catalog getter, so the
// file is scanned for the key. When the file may hold the catalog where a
// scan cannot see it (in a compressed object stream, or encrypted), PDFium
// writes a copy of the document, which has every object in the clear, and
// that is scanned instead.
func (d *pdfiumDocument) hasOptionalContent() bool {
	d.ocOnce.Do(func() { d.hasOC = d.scanOptionalContent() })
	return d.hasOC
}

func (d *pdfiumDocument) scanOptionalContent() bool {
	f, err := os.Open(d.path)
	if err != nil {
		return true // cannot vouch for a document that cannot be read
	}
	defer func() { _ = f.Close() }()
	var s ocScanner
	if _, err := io.Copy(&s, bufio.NewReaderSize(f, 1<<20)); err != nil {
		return true
	}
	switch {
	case s.found["OCProperties"]:
		return true
	case !s.found["ObjStm"] && !s.found["Encrypt"]:
		return false
	}
	var c ocScanner
	_, err = d.instance.FPDF_SaveAsCopy(&requests.FPDF_SaveAsCopy{Document: d.doc.Document, Flags: requests.SaveFlagRemoveSecurity, FileWriter: &c})
	return err != nil || c.found["OCProperties"]
}

// ocScanner is an io.Writer that notes which keys it has seen in the bytes
// written to it, also when a key spans two writes.
type ocScanner struct {
	found map[string]bool
	tail  []byte
}

var ocKeys = []string{"OCProperties", "ObjStm", "Encrypt"}

func (s *ocScanner) Write(p []byte) (int, error) {
	if s.found == nil {
		s.found = map[string]bool{}
	}
	buf := append(s.tail, p...)
	for _, k := range ocKeys {
		if !s.found[k] && bytes.Contains(buf, []byte(k)) {
			s.found[k] = true
		}
	}
	keep := len("OCProperties") - 1
	if len(buf) < keep {
		keep = len(buf)
	}
	s.tail = append([]byte(nil), buf[len(buf)-keep:]...)
	return len(p), nil
}

// errOptionalContent is what ContentHash returns for a document with layers.
var errOptionalContent = fmt.Errorf("%w: document has optional content", ErrNotHashable)
