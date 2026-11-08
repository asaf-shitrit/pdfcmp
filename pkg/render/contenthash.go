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
// out, its rotation and the visible box (CropBox cut to MediaBox); then for every
// page object its type, bounds (rounded to 0.01 pt), transform (the whole
// matrix for images and forms, only mirror and quarter turn for text; for
// paths their segments with points in page space), fill
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
//
// A path also hashes how it is painted: fill rule, whether it is stroked,
// and for a stroke its width, cap, join and dash array and phase; and all of
// its segments, not a sample. An annotation hashes its flags (hidden, print).
// An image hashes its colour space, bit depth and raw data, and, when small,
// the pixels PDFium decodes from it, which carry a /Decode array or a palette.
//
// What the hash cannot capture is not guessed: ContentHash returns
// ErrNotHashable, and the planner renders the page. That covers a document
// with optional content (/OCProperties), as a form or image XObject can be
// a layer through its own /OC entry, which go-pdfium does not report; an
// object (also inside a form) with an optional content mark; a shading object; an image in a colour space other than
// DeviceGray, DeviceRGB or DeviceCMYK that is too large to hash its pixels; and
// a page with more than maxPageSegments path segments. Known gaps, as
// go-pdfium has no getter for them: the blend mode of an ExtGState (/BM), a
// /Decode array on a large image, and an image's soft or stencil mask are not
// hashed, so pages that differ only in these hash equal.
func (d *pdfiumDocument) ContentHash(pageNum int) (uint64, error) {
	if pageNum < 0 || pageNum >= d.pageCount {
		return 0, fmt.Errorf("page %d out of range [0, %d)", pageNum, d.pageCount)
	}
	if d.hasOptionalContent() {
		return 0, errOptionalContent
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
	st := &hashState{}
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
		if err := d.hashObject(h, st, page, objResp.PageObject, 0); err != nil {
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
// one's subtype, rectangle, flags (hidden, print), contents, field value and normal appearance.
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
	if f, err := d.instance.FPDFAnnot_GetFlags(&requests.FPDFAnnot_GetFlags{Annotation: annot}); err == nil {
		writeUint(h, uint64(f.Flags)+1)
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
// the box that is shown, as PDFium resolves it: the CropBox (the MediaBox
// when there is none) cut to the MediaBox, either one inherited from the
// /Pages tree when the page has none of its own (gofpdf writes the MediaBox
// there). A rotated or cropped page shows the same objects differently, so
// its content hash must differ; a page without a CropBox and one whose
// CropBox is its MediaBox look the same, so they hash the same. A value the
// page does not have is part of its identity, not an error.
func (d *pdfiumDocument) hashPageGeometry(h *xxhash.Digest, page requests.Page) {
	if r, err := d.instance.FPDFPage_GetRotation(&requests.FPDFPage_GetRotation{Page: page}); err == nil {
		writeUint(h, uint64(((r.PageRotation%4)+4)%4))
	}
	bb, err := d.instance.FPDF_GetPageBoundingBox(&requests.FPDF_GetPageBoundingBox{Page: page})
	if err != nil {
		writeUint(h, 0)
		return
	}
	for _, v := range []float32{bb.Rect.Left, bb.Rect.Bottom, bb.Rect.Right, bb.Rect.Top} {
		writeCoord(h, v)
	}
	writeUint(h, 0)
}

// hashOrientation writes what a transform matrix shows that an object's
// bounds do not: whether it mirrors (the sign of the determinant) and which
// quarter turn its x axis points to. Scale and shift are in the bounds.
func hashOrientation(h *xxhash.Digest, m [4]float32) {
	a, b, c, d := m[0], m[1], m[2], m[3]
	var flags uint64
	if a*d-b*c < 0 {
		flags |= 1
	}
	ax, ay := math.Abs(float64(a)), math.Abs(float64(b))
	switch {
	case ax >= ay && a >= 0:
	case ax >= ay:
		flags |= 2 << 1
	case b >= 0:
		flags |= 1 << 1
	default:
		flags |= 3 << 1
	}
	writeUint(h, flags+1)
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

// hashPath writes all of a path object's segments into h: their count, and for
// each one read its type, close flag and point mapped through the object
// matrix into page space. Bounds alone do not tell a mirrored or rotated
// drawing from the original; the page-space points do, and they are the
// same however a "cm" splits the transform between the matrix and the points.
func (d *pdfiumDocument) hashPath(h *xxhash.Digest, st *hashState, obj references.FPDF_PAGEOBJECT) error {
	n, err := d.instance.FPDFPath_CountSegments(&requests.FPDFPath_CountSegments{PageObject: obj})
	if err != nil {
		writeUint(h, 0)
		return nil
	}
	if st.segments += n.Count; st.segments > maxPageSegments {
		return fmt.Errorf("%w: more than %d path segments", ErrNotHashable, maxPageSegments)
	}
	writeUint(h, uint64(n.Count)+1)
	var a, b, c, dd, e, f float32 = 1, 0, 0, 1, 0, 0
	if m, err := d.instance.FPDFPageObj_GetMatrix(&requests.FPDFPageObj_GetMatrix{PageObject: obj}); err == nil {
		a, b, c, dd, e, f = m.Matrix.A, m.Matrix.B, m.Matrix.C, m.Matrix.D, m.Matrix.E, m.Matrix.F
	}
	for i := 0; i < n.Count; i++ {
		seg, err := d.instance.FPDFPath_GetPathSegment(&requests.FPDFPath_GetPathSegment{PageObject: obj, Index: i})
		if err != nil {
			continue
		}
		var flags uint64
		if t, err := d.instance.FPDFPathSegment_GetType(&requests.FPDFPathSegment_GetType{PathSegment: seg.PathSegment}); err == nil {
			flags = uint64(t.Type) + 1
		}
		if cl, err := d.instance.FPDFPathSegment_GetClose(&requests.FPDFPathSegment_GetClose{PathSegment: seg.PathSegment}); err == nil && cl.IsClose {
			flags |= 1 << 8
		}
		writeUint(h, flags)
		if pt, err := d.instance.FPDFPathSegment_GetPoint(&requests.FPDFPathSegment_GetPoint{PathSegment: seg.PathSegment}); err == nil {
			writeCoord(h, a*pt.X+c*pt.Y+e)
			writeCoord(h, b*pt.X+dd*pt.Y+f)
		}
	}
	return nil
}

// hashObject writes one page object's identity into h.
func (d *pdfiumDocument) hashObject(h *xxhash.Digest, st *hashState, page requests.Page, obj references.FPDF_PAGEOBJECT, depth int) error {
	typeResp, err := d.instance.FPDFPageObj_GetType(&requests.FPDFPageObj_GetType{PageObject: obj})
	if err != nil {
		return err
	}
	if typeResp.Type == enums.FPDF_PAGEOBJ_SHADING {
		return fmt.Errorf("%w: shading object", ErrNotHashable)
	}
	if err := d.checkOptionalContent(obj); err != nil {
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

	// The matrix tells a flipped image or form from an upright one with the
	// same bounds. For text and paths the bounds already follow the matrix,
	// so scaling the same drawing by a "cm" must not change the hash; text,
	// whose mirror or quarter turn the bounds can hide, adds its orientation,
	// and a path its page-space segments (hashPath). The render mode tells an invisible text object from a
	// visible one; the clip path, an object cut away from one that shows.
	if m, err := d.instance.FPDFPageObj_GetMatrix(&requests.FPDFPageObj_GetMatrix{PageObject: obj}); err == nil {
		switch typeResp.Type {
		case enums.FPDF_PAGEOBJ_IMAGE, enums.FPDF_PAGEOBJ_FORM:
			for _, v := range []float32{m.Matrix.A, m.Matrix.B, m.Matrix.C, m.Matrix.D, m.Matrix.E, m.Matrix.F} {
				writeCoord(h, v)
			}
		case enums.FPDF_PAGEOBJ_TEXT:
			hashOrientation(h, [4]float32{m.Matrix.A, m.Matrix.B, m.Matrix.C, m.Matrix.D})
		}
	}
	if typeResp.Type == enums.FPDF_PAGEOBJ_TEXT {
		if mode, err := d.instance.FPDFTextObj_GetTextRenderMode(&requests.FPDFTextObj_GetTextRenderMode{PageObject: obj}); err == nil {
			writeUint(h, uint64(mode.TextRenderMode)+1)
		}
	}
	if typeResp.Type == enums.FPDF_PAGEOBJ_PATH {
		if err := d.hashPath(h, st, obj); err != nil {
			return err
		}
		d.hashPathStyle(h, obj)
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
		return d.hashForm(h, st, page, obj, depth+1)
	}
	if typeResp.Type != enums.FPDF_PAGEOBJ_IMAGE {
		return nil
	}
	return d.hashImage(h, page, obj)
}

// hashForm writes the objects of a form object into h.
func (d *pdfiumDocument) hashForm(h *xxhash.Digest, st *hashState, page requests.Page, form references.FPDF_PAGEOBJECT, depth int) error {
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
		if err := d.hashObject(h, st, page, obj.PageObject, depth); err != nil {
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
