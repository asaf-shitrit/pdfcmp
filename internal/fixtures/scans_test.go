package fixtures

import (
	"image"
	"image/color"
	"testing"
)

func grey(img image.Image, x, y int) float64 {
	return float64(color.GrayModel.Convert(img.At(x, y)).(color.Gray).Y)
}

func meanAbsDiff(a, b image.Image) float64 {
	sum := 0.0
	r := a.Bounds()
	for y := 0; y < r.Dy(); y += 3 {
		for x := 0; x < r.Dx(); x += 3 {
			d := grey(a, x, y) - grey(b, x, y)
			if d < 0 {
				d = -d
			}
			sum += d
		}
	}
	return sum / float64(r.Dx()/3*r.Dy()/3)
}

func TestRescanDeterministic(t *testing.T) {
	base := ScanImage(1)
	a, b := Rescan(base, 7, RescanMedium), Rescan(base, 7, RescanMedium)
	if d := meanAbsDiff(a, b); d != 0 {
		t.Errorf("same seed gave different rescans, mean difference %v", d)
	}
	if a.Bounds() != base.Bounds() {
		t.Errorf("rescan bounds %v, want %v", a.Bounds(), base.Bounds())
	}
	if d := meanAbsDiff(a, Rescan(base, 8, RescanMedium)); d == 0 {
		t.Error("two seeds gave the same rescan")
	}
}

// Stronger levels stray further from the original.
func TestRescanLevels(t *testing.T) {
	base := ScanImage(1)
	prev := 0.0
	for _, lv := range []RescanLevel{RescanLight, RescanMedium, RescanHeavy} {
		worst := 0.0
		for seed := int64(1); seed <= 3; seed++ {
			worst = max(worst, meanAbsDiff(base, Rescan(base, seed, lv)))
		}
		if worst <= prev {
			t.Errorf("%v: mean difference %.2f, want more than the level before (%.2f)", lv, worst, prev)
		}
		prev = worst
	}
	if RescanHeavy.String() != "heavy" {
		t.Errorf("RescanHeavy.String() = %q", RescanHeavy)
	}
}

func TestMarkImage(t *testing.T) {
	base := ScanImage(1)
	marked := MarkImage(base, 3, image.Pt(100, 100), image.Pt(160, 100), image.Pt(160, 40))
	for _, p := range []image.Point{{100, 100}, {130, 100}, {160, 70}, {160, 40}} {
		if g := grey(marked, p.X, p.Y); g > 25 {
			t.Errorf("pixel %v = %v, want it on the dark stroke", p, g)
		}
	}
	if grey(marked, 300, 300) != grey(base, 300, 300) {
		t.Error("a pixel away from the stroke changed")
	}
	if grey(base, 130, 100) < 40 {
		t.Error("MarkImage drew on its input")
	}
	// strokes at the page edge and single points do not panic
	MarkImage(base, 5, image.Pt(-10, -10), image.Pt(3, 3))
	MarkImage(base, 2, image.Pt(5, 5))
}
