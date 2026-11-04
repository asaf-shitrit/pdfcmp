package content

import "testing"

func FuzzMinHashSimilarity(f *testing.F) {
	f.Add("Invoice total: 1,250.00 EUR", "Invoice total: 1,280.00 EUR")
	f.Add("", "a")
	f.Add("ﬁnal  \t\n résumé —", "final resume")
	f.Fuzz(func(t *testing.T, a, b string) {
		ab, ba := MinHashSimilarity(a, b), MinHashSimilarity(b, a)
		if ab != ba {
			t.Fatalf("not symmetric: %v vs %v", ab, ba)
		}
		if ab < 0 || ab > 1 {
			t.Fatalf("similarity %v outside [0, 1]", ab)
		}
		if s := MinHashSimilarity(a, a); s != 1 {
			t.Fatalf("text against itself: %v, want 1", s)
		}
		sab, okab := Similarity(a, b)
		sba, okba := Similarity(b, a)
		if sab != sba || okab != okba {
			t.Fatalf("Similarity not symmetric: %v,%v vs %v,%v", sab, okab, sba, okba)
		}
		if !okab && sab != 0 {
			t.Fatalf("no estimate, yet similarity %v", sab)
		}
		if len(Shingles(a, DefaultShingleSize)) == 0 && len(Tokens(a)) != 0 {
			t.Fatalf("tokens %q but no shingles", Tokens(a))
		}
	})
}
