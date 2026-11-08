package content

import "testing"

func TestNormalizeNoiseClassesMapEqual(t *testing.T) {
	const want = "The office will find the flow of the affirm offer. Total 17 regions."
	tests := []struct {
		name string
		in   string
	}{
		{"identity", want},
		{"ligatures", "The o\ufb03ce will \ufb01nd the \ufb02ow of the a\ufb03rm o\ufb00er. Total 17 regions."},
		{"soft hyphens", "The of\u00adfice will fi\u00adnd the flow of the affirm offer. Total 17 regions."},
		{"zero width", "The of\u200bfice will\u200c find\u200d the flow\u2060 of the affirm offer.\ufeff Total 17 regions."},
		{"nbsp", "The office\u00a0will find the flow\u202fof the affirm offer. Total\u00a017 regions."},
		{"crlf", "The office will find\r\nthe flow of the affirm offer.\r\nTotal 17 regions.\r\n"},
		{"trailing space before newline", "The office will find \r\nthe flow of the affirm offer. \r\nTotal 17 regions."},
		{"space runs and tabs", "  The office\twill   find the flow of the affirm offer.\n\n\nTotal 17 regions.  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Normalize(tt.in); got != want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.in, got, want)
			}
		})
	}
}

func TestNormalizeUnicodeForms(t *testing.T) {
	// NFC and NFD spellings of "caf\u00e9" must agree.
	if a, b := Normalize("caf\u00e9"), Normalize("cafe\u0301"); a != b {
		t.Errorf("NFC %q != NFD %q", a, b)
	}
}

func TestNormalizeKeepsRealChanges(t *testing.T) {
	base := "Invoice 2024-0917 for 1,299.00 dollars, 17 regions."
	tests := []struct {
		name string
		in   string
	}{
		{"digit", "Invoice 2024-0918 for 1,299.00 dollars, 17 regions."},
		{"word", "Invoice 2024-0917 for 1,299.00 dollar, 17 regions."},
		{"number split", "Invoice 2024-0917 for 1,29 9.00 dollars, 17 regions."},
		{"word order", "Invoice 2024-0917 for dollars 1,299.00, 17 regions."},
		{"hyphen is kept", "Invoice 20240917 for 1,299.00 dollars, 17 regions."},
		{"case", "invoice 2024-0917 for 1,299.00 dollars, 17 regions."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if Normalize(tt.in) == Normalize(base) {
				t.Errorf("Normalize erased the change in %q", tt.in)
			}
		})
	}
}

func TestNormalizeLineEndHyphenKept(t *testing.T) {
	if Normalize("co-\noperate") == Normalize("co-operate") {
		t.Error("line-end hyphen was rejoined; it must not be")
	}
}

func TestNormalizeEmpty(t *testing.T) {
	for _, in := range []string{"", " \r\n\t", "\u00ad\u200b"} {
		if got := Normalize(in); got != "" {
			t.Errorf("Normalize(%q) = %q, want empty", in, got)
		}
	}
}

func TestBag(t *testing.T) {
	if Bag("be perfect\nThe Law") != Bag("The Law beperfect") {
		t.Error("reordered runs and moved spaces change the bag")
	}
	if Bag("• item 1200") != Bag("ï item 1200") || Bag("a\x05b") != Bag("ab") {
		t.Error("a glyph mapped to another symbol, or a control character, changes the bag")
	}
	for _, other := range []string{"The Law be perfekt", "The Law be perfec", "The Law be perfect 2", "Закон"} {
		if Bag("be perfect The Law") == Bag(other) {
			t.Errorf("%q has the same bag", other)
		}
	}
}

func TestSwapped(t *testing.T) {
	for _, tc := range []struct {
		name, a, b string
		want       bool
	}{
		{"comma and period in a number", "due 1,500 USD", "due 1.500 USD", true},
		{"time", "at 12:30", "at 12.30", true},
		{"u to o umlaut", "Müller", "Möller", true},
		{"same text", "due 1,500 USD", "due 1,500 USD", false},
		{"punctuation outside a number", "due: 1500", "due; 1500", false},
		{"bullet remapped to i diaeresis", "• item 1200", "ï item 1200", false},
		{"glyphs remapped to letters", "• a • b", "í a ñ b", false},
		{"space moved after a period", "in 2035. 3 City", "in 2035.3 City", false},
		{"bullet to symbol", "• item", "¥ item", false},
		{"spaces moved opposite ways", "Total 2035. 3 rows, 7,5", "Total 2035.3 rows, 7, 5", false},
		{"two moved spaces", "items 1, 2 and 3.4", "items 1,2 and 3. 4", false},
		{"isolated glyph remap", "Þ item", "ß item", false},
		{"swap with a moved space", "due 1,500 USD", "due 1. 500 USD", true},
		{"swap beside a moved space", "Amount due: 1,500 USD", "Amount due:1.500 USD", true},
		{"swap beside a bullet remap", "• Total 1,500 EUR", "¥ Total 1.500 EUR", true},
		{"swap beside an i diaeresis remap", "• Total 1,500 EUR", "ï Total 1.500 EUR", true},
		{"swap beside a counted change", "• Total 1,500 EUR", "¥ Total 1.600 EUR", false},
		{"accent swap with a moved space", "55-718 café EUR 75.227", "55-718caf å EUR 75.227", true},
		{"accent swap, space moved beside it", "Müller AG", "M ö ller AG", true},
		{"moved space only, accented", "55-718 café EUR", "55-718caf é EUR", false},
		{"different length", "Müller", "Möller x", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Swapped(tc.a, tc.b); got != tc.want {
				t.Errorf("Swapped(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}
