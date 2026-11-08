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

// ContentHash hashes a page's graphical content: first how the page is laid
// out, its rotation and its media and crop boxes; then for every
// page object its type, bounds (rounded to 0.01 pt), transform matrix, fill
// and stroke colour, clip path, for text objects the render mode, and for
// image objects the raw image data, down into form objects, where PDF
// writers such as LaTeX put included figures; then the page's annotations,
// which are drawn with it: comments, stamps and form fields. Bounds alone
// do not say how an object is drawn: a flipped image has the same bounds, an
// invisible text object (render mode 3) has the same text, and a clip path
// can hide everything under it, and the planner scores a page whose hashes
// match as identical without rendering it. So the hash covers these too, and
// a rotated or cropped page hashes differently from the original. Two pages
// with the same text and object count but redrawn graphics, a replaced
// image or an added annotation hash differently.
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

	h := xxhash.New()
	d.hashPageGeometry(h, page)

	countResp, err := d.instance.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{Page: page})
	if err != nil {
		return 0, fmt.Errorf("failed to count objects: %w", err)
	}
	for i := 0; i < countResp.Count; i++ {
		objResp, err := d.instance.FPDFPage_GetObject(&requests.FPDFPage_GetObject{Page: page, Index: i})
		if err != nil {
			return 0, fmt.Errorf("failed to get object %d: %w", i, err)
		}
		if err := d.hashObject(h, objResp.PageObject, 0); err != nil {
			return 0, fmt.Errorf("object %d: %w", i, err)
		}
	}
	if err := d.hashAnnotations(h, page); err != nil {
		return 0, err
	}
	return h.Sum64(), nil
}

// maxFormDepth bounds how deep hashObject follows form objects nested in
// form objects, against a document whose forms contain each other.
const maxFormDepth = 16

// hashAnnotations writes the identity of a page's annotations into h: each
// one's subtype, rectangle, contents, field value and normal appearance.
func (d *pdfiumDocument) hashAnnotations(h *xxhash.Digest, page requests.Page) error {
	count, err := d.instance.FPDFPage_GetAnnotCount(&requests.FPDFPage_GetAnnotCount{Page: page})
	if err != nil {
		return fmt.Errorf("failed to count annotations: %w", err)
	}
	writeUint(h, uint64(count.Count))
	for i := 0; i < count.Count; i++ {
		annot, err := d.instance.FPDFPage_GetAnnot(&requests.FPDFPage_GetAnnot{Page: page, Index: i})
		if err != nil {
			return fmt.Errorf("failed to get annotation %d: %w", i, err)
		}
		d.hashAnnotation(h, annot.Annotation)
		_, _ = d.instance.FPDFPage_CloseAnnot(&requests.FPDFPage_CloseAnnot{Annotation: annot.Annotation})
	}
	return nil
}

// hashAnnotation writes one annotation's identity into h. A value the
// annotation does not have is part of its identity, not an error.
func (d *pdfiumDocument) hashAnnotation(h *xxhash.Digest, annot references.FPDF_ANNOTATION) {
	if sub, err := d.instance.FPDFAnnot_GetSubtype(&requests.FPDFAnnot_GetSubtype{Annotation: annot}); err == nil {
		writeUint(h, uint64(sub.Subtype))
	}
	if r, err := d.instance.FPDFAnnot_GetRect(&requests.FPDFAnnot_GetRect{Annotation: annot}); err == nil {
		for _, v := range []float32{r.Rect.Left, r.Rect.Bottom, r.Rect.Right, r.Rect.Top} {
			writeCoord(h, v)
		}
	}
	for _, key := range []string{"Contents", "V"} {
		if v, err := d.instance.FPDFAnnot_GetStringValue(&requests.FPDFAnnot_GetStringValue{Annotation: annot, Key: key}); err == nil {
			_, _ = h.WriteString(v.Value)
		}
		writeUint(h, 0)
	}
	if ap, err := d.instance.FPDFAnnot_GetAP(&requests.FPDFAnnot_GetAP{Annotation: annot, AppearanceMode: enums.FPDF_ANNOT_APPEARANCEMODE_NORMAL}); err == nil {
		_, _ = h.WriteString(ap.Value)
	}
}

// writeCoord writes a coordinate rounded to 0.01 pt into h.
func writeCoord(h *xxhash.Digest, v float32) {
	writeUint(h, uint64(int64(math.Round(float64(v)*100))))
}

// hashPageGeometry writes how the page is laid out into h: its rotation and
// media box and its crop box. A rotated or cropped page shows the same objects differently, so its
// content hash must differ. A value the page does not have is part of its
// identity, not an error.
func (d *pdfiumDocument) hashPageGeometry(h *xxhash.Digest, page requests.Page) {
	if r, err := d.instance.FPDFPage_GetRotation(&requests.FPDFPage_GetRotation{Page: page}); err == nil {
		writeUint(h, uint64(r.PageRotation))
	}
	media, mediaErr := d.instance.FPDFPage_GetMediaBox(&requests.FPDFPage_GetMediaBox{Page: page})
	if mediaErr == nil {
		for _, v := range []float32{media.Left, media.Bottom, media.Right, media.Top} {
			writeCoord(h, v)
		}
	}
	writeUint(h, 0)
	if crop, err := d.instance.FPDFPage_GetCropBox(&requests.FPDFPage_GetCropBox{Page: page}); err == nil {
		for _, v := range []float32{crop.Left, crop.Bottom, crop.Right, crop.Top} {
			writeCoord(h, v)
		}
	}
	writeUint(h, 0)
}

// hashClipPath writes the clip path of a page object into h: the number of
// paths, and for each its segments' types and points. An object without a
// clip path is part of its identity too, not an error.
func (d *pdfiumDocument) hashClipPath(h *xxhash.Digest, obj references.FPDF_PAGEOBJECT) {
	clip, err := d.instance.FPDFPageObj_GetClipPath(&requests.FPDFPageObj_GetClipPath{PageObject: obj})
	if err != nil || clip.ClipPath == "" {
		writeUint(h, 0)
		return
	}
	paths, err := d.instance.FPDFClipPath_CountPaths(&requests.FPDFClipPath_CountPaths{ClipPath: clip.ClipPath})
	if err != nil {
		writeUint(h, 0)
		return
	}
	writeUint(h, uint64(paths.Count)+1)
	for p := 0; p < paths.Count; p++ {
		segs, err := d.instance.FPDFClipPath_CountPathSegments(&requests.FPDFClipPath_CountPathSegments{ClipPath: clip.ClipPath, PathIndex: p})
		if err != nil {
			writeUint(h, 0)
			continue
		}
		writeUint(h, uint64(segs.Count)+1)
		for s := 0; s < segs.Count; s++ {
			seg, err := d.instance.FPDFClipPath_GetPathSegment(&requests.FPDFClipPath_GetPathSegment{ClipPath: clip.ClipPath, PathIndex: p, SegmentIndex: s})
			if err != nil {
				continue
			}
			if t, err := d.instance.FPDFPathSegment_GetType(&requests.FPDFPathSegment_GetType{PathSegment: seg.PathSegment}); err == nil {
				writeUint(h, uint64(t.Type))
			}
			if pt, err := d.instance.FPDFPathSegment_GetPoint(&requests.FPDFPathSegment_GetPoint{PathSegment: seg.PathSegment}); err == nil {
				writeCoord(h, pt.X)
				writeCoord(h, pt.Y)
			}
		}
	}
}

// hashObject writes one page object's identity into h.
func (d *pdfiumDocument) hashObject(h *xxhash.Digest, obj references.FPDF_PAGEOBJECT, depth int) error {
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
		writeCoord(h, v)
	}

	// The matrix tells a flipped or mirrored object from an upright one with
	// the same bounds; the render mode, an invisible text object from a
	// visible one; the clip path, an object cut away from one that shows.
	if m, err := d.instance.FPDFPageObj_GetMatrix(&requests.FPDFPageObj_GetMatrix{PageObject: obj}); err == nil {
		for _, v := range []float32{m.Matrix.A, m.Matrix.B, m.Matrix.C, m.Matrix.D, m.Matrix.E, m.Matrix.F} {
			writeCoord(h, v)
		}
	}
	if typeResp.Type == enums.FPDF_PAGEOBJ_TEXT {
		if mode, err := d.instance.FPDFTextObj_GetTextRenderMode(&requests.FPDFTextObj_GetTextRenderMode{PageObject: obj}); err == nil {
			writeUint(h, uint64(mode.TextRenderMode)+1)
		}
	}
	d.hashClipPath(h, obj)

	// A colour the object does not have is part of its identity, not an error.
	if c, err := d.instance.FPDFPageObj_GetFillColor(&requests.FPDFPageObj_GetFillColor{PageObject: obj}); err == nil {
		writeUint(h, uint64(c.FillColor.R)<<24|uint64(c.FillColor.G)<<16|uint64(c.FillColor.B)<<8|uint64(c.FillColor.A))
	}
	if c, err := d.instance.FPDFPageObj_GetStrokeColor(&requests.FPDFPageObj_GetStrokeColor{PageObject: obj}); err == nil {
		writeUint(h, uint64(c.StrokeColor.R)<<24|uint64(c.StrokeColor.G)<<16|uint64(c.StrokeColor.B)<<8|uint64(c.StrokeColor.A))
	}

	if typeResp.Type == enums.FPDF_PAGEOBJ_FORM && depth < maxFormDepth {
		return d.hashForm(h, obj, depth+1)
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

// hashForm writes the objects of a form object into h.
func (d *pdfiumDocument) hashForm(h *xxhash.Digest, form references.FPDF_PAGEOBJECT, depth int) error {
	count, err := d.instance.FPDFFormObj_CountObjects(&requests.FPDFFormObj_CountObjects{PageObject: form})
	if err != nil {
		return err
	}
	writeUint(h, uint64(count.Count))
	for i := 0; i < count.Count; i++ {
		obj, err := d.instance.FPDFFormObj_GetObject(&requests.FPDFFormObj_GetObject{PageObject: form, Index: uint64(i)})
		if err != nil {
			return err
		}
		if err := d.hashObject(h, obj.PageObject, depth); err != nil {
			return err
		}
	}
	return nil
}

func writeUint(h *xxhash.Digest, v uint64) {
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], v)
	_, _ = h.Write(buf[:])
}
