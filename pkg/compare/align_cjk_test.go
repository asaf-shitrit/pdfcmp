package compare

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// chapters builds one short page per chapter id. Each has text of its own and
// a footer that counts pages, so deleting a chapter changes every footer.
// english spaces its words; otherwise the page is Chinese, written without
// spaces.
func chapters(english bool, ids ...int) map[int]string {
	text := map[int]string{}
	for k, id := range ids {
		var b strings.Builder
		for i := range 12 {
			if english {
				fmt.Fprintf(&b, "word%c%c ", rune('a'+id), rune('a'+i))
			} else {
				b.WriteRune(rune(0x4e00 + id*40 + i))
			}
		}
		if english {
			fmt.Fprintf(&b, "\nPage %d of %d", k+1, len(ids))
		} else {
			fmt.Fprintf(&b, "第%d页共%d页", k+1, len(ids))
		}
		text[k] = b.String()
	}
	return text
}

// A page with Chinese text is paired as an English one is: deleting chapter 2
// leaves chapters 1 and 3 paired with their own counterparts.
func TestAlignChineseDeletedPage(t *testing.T) {
	for _, english := range []bool{true, false} {
		res := alignFake(t, chapters(english, 1, 2, 3), chapters(english, 1, 3))
		if !slices.Equal(res.DeletedPages, []int{2}) || len(res.InsertedPages) != 0 {
			t.Errorf("english=%v: deleted %v inserted %v, want [2] and none", english, res.DeletedPages, res.InsertedPages)
		}
	}
}

func TestPairGapChinese(t *testing.T) {
	for _, english := range []bool{true, false} {
		a, b := preparePages(chapters(english, 1, 2, 3)), preparePages(chapters(english, 1, 3))
		got := pairGap(3, 2, func(i, j int) float64 { return textLikeness(a[i], b[j]) })
		want := []pagePair{{0, 0}, {2, 1}}
		if !slices.Equal(got, want) {
			t.Errorf("english=%v: pairGap = %v, want %v", english, got, want)
		}
	}
}

func preparePages(m map[int]string) []*pageText {
	out := make([]*pageText, len(m))
	for i := range out {
		out[i] = newPageText(m[i])
	}
	return out
}

// longPages builds one page of 800 words per id, with a footer that counts
// the pages.
func longPages(ids []int) map[int]string {
	text := map[int]string{}
	for k, id := range ids {
		var b strings.Builder
		for i := range 800 {
			fmt.Fprintf(&b, "w%d_%d ", id, i%400)
		}
		fmt.Fprintf(&b, "page %d of %d", k+1, len(ids))
		text[k] = b.String()
	}
	return text
}

// Aligning 99 pages with 100 pages whose footers all differ pairs one gap of
// 99 by 100 pages, which is 99*2 text comparisons per page: each page is
// prepared once, not once per comparison.
func BenchmarkAlignLongPages(b *testing.B) {
	ids := make([]int, 99)
	for i := range ids {
		ids[i] = i
	}
	t1 := longPages(ids)
	t2 := longPages(slices.Insert(slices.Clone(ids), 50, 1000))
	b.ResetTimer()
	for range b.N {
		res := alignFake(b, t1, t2)
		if len(res.InsertedPages) != 1 || res.InsertedPages[0] != 51 {
			b.Fatalf("inserted %v, want [51]", res.InsertedPages)
		}
	}
}
