package content

import (
	"slices"
	"strings"
	"testing"
)

func TestTokensUnspacedScripts(t *testing.T) {
	tests := []struct {
		name, text string
		want       []string
	}{
		{"han", "你好世界", []string{"你", "好", "世", "界"}},
		{"han with punctuation", "你好，世界。", []string{"你", "好", "世", "界"}},
		{"hiragana and katakana", "こんにちはカタカナ", []string{"こ", "ん", "に", "ち", "は", "カ", "タ", "カ", "ナ"}},
		{"hangul", "안녕 하세요", []string{"안", "녕", "하", "세", "요"}},
		{"thai", "สวัสดี", []string{"ส", "ว", "ั", "ส", "ด", "ี"}},
		{"mixed with latin", "Page 3 第一章 Intro.", []string{"page", "3", "第", "一", "章", "intro"}},
		{"latin run next to han", "PDF文件v2", []string{"pdf", "文", "件", "v2"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Tokens(tc.text); !slices.Equal(got, tc.want) {
				t.Errorf("Tokens(%q) = %q, want %q", tc.text, got, tc.want)
			}
		})
	}
}

// Text without a character of an unspaced script splits on whitespace alone,
// as it did before those scripts were split.
func TestTokensSpacedTextUnchanged(t *testing.T) {
	tests := []struct {
		text string
		want []string
	}{
		{"Hello, World! (1,500) -- Müller's café", []string{"hello", "world", "1,500", "müller's", "café"}},
		{"  a\tb\n\nc  ", []string{"a", "b", "c"}},
		{"Привет, мир! Ελληνικά; עברית العربية", []string{"привет", "мир", "ελληνικά", "עברית", "العربية"}},
		{"... --- !!!", []string{}},
		{"", []string{}},
	}
	for _, tc := range tests {
		if got := Tokens(tc.text); !slices.Equal(got, tc.want) {
			t.Errorf("Tokens(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

func chinesePage(n int) string {
	var b strings.Builder
	for i := range 300 {
		b.WriteRune(rune(0x4e00 + (i*7+n*131)%1500))
		if i%20 == 19 {
			b.WriteString("。\n")
		}
	}
	return b.String()
}

func TestSimilarityChinese(t *testing.T) {
	a := chinesePage(1)
	near := strings.Replace(a, string([]rune(a)[10]), "X", 1)
	s, ok := Similarity(a, near)
	if !ok || s < 0.8 {
		t.Errorf("near-identical Chinese pages: s=%v ok=%v, want ok and >= 0.8", s, ok)
	}
	s, ok = Similarity(a, chinesePage(2))
	if !ok || s > 0.2 {
		t.Errorf("different Chinese pages: s=%v ok=%v, want ok and <= 0.2", s, ok)
	}
}

func TestSignatureMatchesSimilarity(t *testing.T) {
	a, b := chinesePage(1), chinesePage(1)+"追加"
	sa, sb := NewSignature(Tokens(a)), NewSignature(Tokens(b))
	got, gok := sa.Similarity(&sb)
	want, wok := Similarity(a, b)
	if got != want || gok != wok {
		t.Errorf("Signature.Similarity = %v,%v, Similarity = %v,%v", got, gok, want, wok)
	}
}
