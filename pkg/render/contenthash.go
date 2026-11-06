package render

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/cespare/xxhash/v2"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
)

// ContentHash hashes a page's graphical content: for every page object its
// type, bounds (rounded to 0.01 pt), fill and stroke colour, and for image
// objects the raw image data. Two pages with the same text and object count
// but redrawn graphics hash differently.
func (d *pdfiumDocument) ContentHash(pageNum int) (uint64, error) {
	if pageNum < 0 || pageNum >= d.pageCount {
		return 0, fmt.Errorf("page %d out of range [0, %d)", pageNum, d.pageCount)
	}
	loaded, err := d.instance.FPDF_LoadPage(&requests.FPDF_LoadPage{Document: d.doc.Document, Index: pageNum})
	if err != nil {
		return 0, fmt.Errorf("failed to load page: %w", err)
	}
	defer func() {
		_, _ = d.instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: loaded.Page})
	}()
	page := requests.Page{ByReference: &loaded.Page}

	countResp, err := d.instance.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{Page: page})
	if err != nil {
		return 0, fmt.Errorf("failed to count objects: %w", err)
	}
	h := xxhash.New()
	for i := 0; i < countResp.Count; i++ {
		objResp, err := d.instance.FPDFPage_GetObject(&requests.FPDFPage_GetObject{Page: page, Index: i})
		if err != nil {
			return 0, fmt.Errorf("failed to get object %d: %w", i, err)
		}
		if err := d.hashObject(h, objResp.PageObject); err != nil {
			return 0, fmt.Errorf("object %d: %w", i, err)
		}
	}
	return h.Sum64(), nil
}

// hashObject writes one page object's identity into h.
func (d *pdfiumDocument) hashObject(h *xxhash.Digest, obj references.FPDF_PAGEOBJECT) error {
	typeResp, err := d.instance.FPDFPageObj_GetType(&requests.FPDFPageObj_GetType{PageObject: obj})
	if err != nil {
		return err
	}
	writeUint(h, uint64(typeResp.Type))

	b, err := d.instance.FPDFPageObj_GetBounds(&requests.FPDFPageObj_GetBounds{PageObject: obj})
	if err != nil {
		return err
	}
	for _, v := range []float32{b.Left, b.Bottom, b.Right, b.Top} {
		writeUint(h, uint64(int64(math.Round(float64(v)*100))))
	}

	// A colour the object does not have is part of its identity, not an error.
	if c, err := d.instance.FPDFPageObj_GetFillColor(&requests.FPDFPageObj_GetFillColor{PageObject: obj}); err == nil {
		writeUint(h, uint64(c.FillColor.R)<<24|uint64(c.FillColor.G)<<16|uint64(c.FillColor.B)<<8|uint64(c.FillColor.A))
	}
	if c, err := d.instance.FPDFPageObj_GetStrokeColor(&requests.FPDFPageObj_GetStrokeColor{PageObject: obj}); err == nil {
		writeUint(h, uint64(c.StrokeColor.R)<<24|uint64(c.StrokeColor.G)<<16|uint64(c.StrokeColor.B)<<8|uint64(c.StrokeColor.A))
	}

	if typeResp.Type != enums.FPDF_PAGEOBJ_IMAGE {
		return nil
	}
	img, err := d.instance.FPDFImageObj_GetImageDataRaw(&requests.FPDFImageObj_GetImageDataRaw{ImageObject: obj})
	if err != nil {
		return err
	}
	_, _ = h.Write(img.Data)
	return nil
}

func writeUint(h *xxhash.Digest, v uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], v)
	_, _ = h.Write(buf[:])
}
