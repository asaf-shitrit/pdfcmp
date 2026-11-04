package content

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
	"strings"
	"testing"
)

func words(seed int64, n int) []string {
	r := rand.New(rand.NewSource(seed))
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("w%d", r.Intn(5000))
	}
	return out
}

// exactJaccard is the Jaccard similarity of the two texts' shingle sets.
func exactJaccard(a, b string) float64 {
	set := func(s string) map[uint64]bool {
		m := map[uint64]bool{}
		for _, h := range Shingles(s, DefaultShingleSize) {
			m[h] = true
		}
		return m
	}
	sa, sb := set(a), set(b)
	inter := 0
	for h := range sa {
		if sb[h] {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return 1
	}
	return float64(inter) / float64(union)
}

func TestTokens(t *testing.T) {
	got := Tokens("Invoice total: 1,250.00 EUR.\n\t-- Due  (now)")
	want := []string{"invoice", "total", "1,250.00", "eur", "due", "now"}
	if !slices.Equal(got, want) {
		t.Fatalf("Tokens = %q, want %q", got, want)
	}
}

func TestShingles(t *testing.T) {
	if got := Shingles("  \n ", 3); got != nil {
		t.Fatalf("blank text: got %v, want nil", got)
	}
	if got := len(Shingles("one two", 3)); got != 1 {
		t.Fatalf("short text: got %d shingles, want 1", got)
	}
	if got := len(Shingles("a b c d e", 3)); got != 3 {
		t.Fatalf("five tokens, k=3: got %d shingles, want 3", got)
	}
	if Shingles("a b c", 3)[0] == Shingles("c b a", 3)[0] {
		t.Fatal("shingle hash ignores token order")
	}
	if !slices.Equal(Shingles("A b, c", 3), Shingles("a B c", 3)) {
		t.Fatal("case or edge punctuation changes the shingle")
	}
}

func TestMinHashTracksJaccard(t *testing.T) {
	base := words(1, 250)
	for _, changed := range []int{0, 1, 10, 50, 125, 250} {
		other := slices.Clone(base)
		repl := words(int64(100+changed), changed)
		copy(other[len(other)-changed:], repl)

		a, b := strings.Join(base, " "), strings.Join(other, " ")
		j := exactJaccard(a, b)
		est := MinHashSimilarity(a, b)
		// Four standard errors, plus one slot for rounding.
		tol := 4*math.Sqrt(j*(1-j)/MinHashSize) + 1.0/MinHashSize
		if math.Abs(est-j) > tol {
			t.Errorf("changed %d of 250 tokens: MinHash %.3f, Jaccard %.3f, tolerance %.3f", changed, est, j, tol)
		}
	}
}

func TestMinHashEdges(t *testing.T) {
	text := strings.Join(words(2, 200), " ")
	if s := MinHashSimilarity(text, text); s != 1 {
		t.Fatalf("identical text: %v, want 1", s)
	}
	unrelated := strings.Join(words(3, 200), " ")
	if s := MinHashSimilarity(text, unrelated); s > 0.05 {
		t.Fatalf("unrelated text: %v, want near 0", s)
	}
	if MinHashSimilarity(text, unrelated) != MinHashSimilarity(unrelated, text) {
		t.Fatal("MinHash similarity is not symmetric")
	}
	if s := MinHashSimilarity("", text); s != 0 {
		t.Fatalf("empty vs text: %v, want 0", s)
	}
}

func BenchmarkMinHashPage(b *testing.B) {
	shingles := Shingles(strings.Join(words(6, 250), " "), DefaultShingleSize)
	b.ResetTimer()
	for b.Loop() {
		_ = NewMinHash(shingles)
	}
}

func BenchmarkShinglesPage(b *testing.B) {
	text := strings.Join(words(6, 250), " ")
	for b.Loop() {
		_ = Shingles(text, DefaultShingleSize)
	}
}

func TestSimilarityNeedsEnoughShingles(t *testing.T) {
	if _, ok := Similarity("Annual Report 2025", "Annual Report 2026"); ok {
		t.Error("three-word title: want no estimate")
	}

	long := strings.Join(words(7, MinShingles+2), " ") // MinShingles distinct shingles
	other := strings.Fields(long)
	other[0] = "changed"
	s, ok := Similarity(long, strings.Join(other, " "))
	if !ok {
		t.Fatalf("%d-token text: want an estimate", MinShingles+2)
	}
	if want := MinHashSimilarity(long, strings.Join(other, " ")); s != want {
		t.Errorf("Similarity = %v, want MinHashSimilarity's %v", s, want)
	}

	repeated := strings.Repeat("same three words ", 100)
	if _, ok := Similarity(repeated, repeated+" more"); ok {
		t.Error("100 repeats of one phrase: want no estimate, it has few distinct shingles")
	}

	// A long page replaced by a short notice: one side is long enough, and
	// the low estimate is the point.
	s, ok = Similarity(long, "THIS PAGE HAS BEEN MODIFIED FOR TESTING")
	if !ok || s > 0.1 {
		t.Errorf("long page against a short notice: got %v, %v; want an estimate near 0", s, ok)
	}
}
