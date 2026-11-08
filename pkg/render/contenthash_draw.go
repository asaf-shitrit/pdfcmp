package render

import (
	"errors"
	"fmt"

	"github.com/cespare/xxhash/v2"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
)

// ErrNotHashable is returned by ContentHash for a page whose appearance the
// hash cannot capture: the page has optional content (layers), shading
// objects, an image whose colours or masks the hash cannot read, or more
// path segments than hashing is worth. The hash is an optimisation that lets
// the planner skip rendering, so a page it cannot vouch for is rendered.
var ErrNotHashable = errors.New("page content cannot be hashed")

// maxPageSegments bounds the path segments hashed on one page, at about
// 1.4 us each: beyond it rendering the page is not much dearer than hashing
// it, and the page is not hashable.
const maxPageSegments = 200_000

// maxPixelHashArea is the largest image, in pixels, whose decoded pixels are
// hashed; copying a larger one out of PDFium costs more than it saves.
const maxPixelHashArea = 65_536

// hashState is what one ContentHash call accumulates across objects.
type hashState struct {
	segments int // path segments read so far
}

// checkOptionalContent returns ErrNotHashable when the object carries an
// optional content mark ("OC"): a layer that a viewer can switch on and off,
// which the object's own data does not say.
func (d *pdfiumDocument) checkOptionalContent(obj references.FPDF_PAGEOBJECT) error {
	n, err := d.instance.FPDFPageObj_CountMarks(&requests.FPDFPageObj_CountMarks{PageObject: obj})
	if err != nil {
		return fmt.Errorf("%w: marks: %v", ErrNotHashable, err)
	}
	for i := 0; i < n.Count; i++ {
		mark, err := d.instance.FPDFPageObj_GetMark(&requests.FPDFPageObj_GetMark{PageObject: obj, Index: uint64(i)})
		if err != nil {
			return fmt.Errorf("%w: mark: %v", ErrNotHashable, err)
		}
		name, err := d.instance.FPDFPageObjMark_GetName(&requests.FPDFPageObjMark_GetName{PageObjectMark: mark.Mark})
		if err != nil || name.Name == "OC" {
			return fmt.Errorf("%w: optional content", ErrNotHashable)
		}
	}
	return nil
}

// hashPathStyle writes how a path object is painted into h: fill rule and
// stroke flag, and for a stroked path its width, line cap and join, and dash
// array and phase. "re S" and "re B" have the same segments and colours.
func (d *pdfiumDocument) hashPathStyle(h *xxhash.Digest, obj references.FPDF_PAGEOBJECT) {
	mode, err := d.instance.FPDFPath_GetDrawMode(&requests.FPDFPath_GetDrawMode{PageObject: obj})
	if err != nil {
		writeUint(h, 0)
		return
	}
	flags := uint64(mode.FillMode) + 1
	if mode.Stroke {
		flags |= 1 << 8
	}
	writeUint(h, flags)
	if !mode.Stroke {
		return
	}
	if w, err := d.instance.FPDFPageObj_GetStrokeWidth(&requests.FPDFPageObj_GetStrokeWidth{PageObject: obj}); err == nil {
		writeCoord(h, w.StrokeWidth)
	}
	if c, err := d.instance.FPDFPageObj_GetLineCap(&requests.FPDFPageObj_GetLineCap{PageObject: obj}); err == nil {
		writeUint(h, uint64(c.LineCap)+1)
	}
	if j, err := d.instance.FPDFPageObj_GetLineJoin(&requests.FPDFPageObj_GetLineJoin{PageObject: obj}); err == nil {
		writeUint(h, uint64(j.LineJoin)+1)
	}
	d.hashDash(h, obj)
}

// hashDash writes a path's dash array and phase into h.
func (d *pdfiumDocument) hashDash(h *xxhash.Digest, obj references.FPDF_PAGEOBJECT) {
	n, err := d.instance.FPDFPageObj_GetDashCount(&requests.FPDFPageObj_GetDashCount{PageObject: obj})
	if err != nil || n.DashCount <= 0 {
		writeUint(h, 0)
		return
	}
	writeUint(h, uint64(n.DashCount)+1)
	if arr, err := d.instance.FPDFPageObj_GetDashArray(&requests.FPDFPageObj_GetDashArray{PageObject: obj}); err == nil {
		for _, v := range arr.DashArray {
			writeCoord(h, v)
		}
	}
	if p, err := d.instance.FPDFPageObj_GetDashPhase(&requests.FPDFPageObj_GetDashPhase{PageObject: obj}); err == nil {
		writeCoord(h, p.DashPhase)
	}
}

// hashImage writes an image object's identity into h: its raw data, bit
// depth and colour space. Raw data does not say how it is decoded: a /Decode
// array or an /Indexed palette changes the look of the same bytes, and
// go-pdfium does not report either. So an image of at most maxPixelHashArea
// pixels also hashes the pixels PDFium decodes from it, which carry both; a
// larger image must be in DeviceGray, DeviceRGB or DeviceCMYK or it is
// ErrNotHashable (an /Indexed palette or ICC profile cannot be told from the
// raw data). Known gap: a /Decode array on a large device-colour image, and
// a soft mask or stencil mask on any image, are not hashed.
func (d *pdfiumDocument) hashImage(h *xxhash.Digest, page requests.Page, obj references.FPDF_PAGEOBJECT) error {
	meta, err := d.instance.FPDFImageObj_GetImageMetadata(&requests.FPDFImageObj_GetImageMetadata{ImageObject: obj, Page: page})
	if err != nil {
		return fmt.Errorf("%w: image metadata: %v", ErrNotHashable, err)
	}
	m := meta.ImageMetadata
	writeUint(h, uint64(m.Colorspace))
	writeUint(h, uint64(m.BitsPerPixel))
	writeUint(h, uint64(m.Width)<<32|uint64(m.Height))

	img, err := d.instance.FPDFImageObj_GetImageDataRaw(&requests.FPDFImageObj_GetImageDataRaw{ImageObject: obj})
	if err != nil {
		return err
	}
	_, _ = h.Write(img.Data)
	if m.Width*m.Height <= maxPixelHashArea {
		return d.hashImagePixels(h, obj)
	}
	switch m.Colorspace {
	case enums.FPDF_COLORSPACE_DEVICEGRAY, enums.FPDF_COLORSPACE_DEVICERGB, enums.FPDF_COLORSPACE_DEVICECMYK:
		return nil
	}
	return fmt.Errorf("%w: large image in colour space %d", ErrNotHashable, m.Colorspace)
}

// hashImagePixels writes the pixels PDFium decodes from an image object
// (before its matrix and mask) into h.
func (d *pdfiumDocument) hashImagePixels(h *xxhash.Digest, obj references.FPDF_PAGEOBJECT) error {
	bmp, err := d.instance.FPDFImageObj_GetBitmap(&requests.FPDFImageObj_GetBitmap{ImageObject: obj})
	if err != nil {
		return fmt.Errorf("%w: image bitmap: %v", ErrNotHashable, err)
	}
	defer func() {
		_, _ = d.instance.FPDFBitmap_Destroy(&requests.FPDFBitmap_Destroy{Bitmap: bmp.Bitmap})
	}()
	buf, err := d.instance.FPDFBitmap_GetBuffer(&requests.FPDFBitmap_GetBuffer{Bitmap: bmp.Bitmap})
	if err != nil {
		return fmt.Errorf("%w: image pixels: %v", ErrNotHashable, err)
	}
	_, _ = h.Write(buf.Buffer)
	return nil
}
