package ui

import (
	"reflect"
	"testing"
)

func TestExtractKeyValuePairs_dotPrefixMasked(t *testing.T) {
	pairs, rest := ExtractKeyValuePairs(".secret: myvalue\n")
	if rest != "" {
		t.Fatalf("rest: got %q want empty", rest)
	}
	if len(pairs) != 1 {
		t.Fatalf("len(pairs)=%d want 1", len(pairs))
	}
	if pairs[0].Key != ".secret" || pairs[0].Value != "myvalue" || !pairs[0].IsMasked {
		t.Fatalf("pair: %+v", pairs[0])
	}
}

func TestExtractKeyValuePairs_plainKeysUnmasked(t *testing.T) {
	input := "secret: a\npassword: sec\npin: 1234\n"
	pairs, rest := ExtractKeyValuePairs(input)
	if rest != "" {
		t.Fatalf("rest: got %q want empty", rest)
	}
	want := []KeyValuePair{
		{Key: "secret", Value: "a", IsMasked: false},
		{Key: "password", Value: "sec", IsMasked: false},
		{Key: "pin", Value: "1234", IsMasked: false},
	}
	if !reflect.DeepEqual(pairs, want) {
		t.Fatalf("pairs:\n got %+v\nwant %+v", pairs, want)
	}
}

func TestExtractKeyValuePairs_maskedUnmaskedNotSpecial(t *testing.T) {
	input := "card: 4111\nmasked: pin, foo\nunmasked: bar\n.pin: hidden\n"
	pairs, rest := ExtractKeyValuePairs(input)
	if rest != "" {
		t.Fatalf("rest: got %q want empty", rest)
	}
	if len(pairs) != 4 {
		t.Fatalf("len(pairs)=%d want 4", len(pairs))
	}
	for _, p := range pairs {
		switch p.Key {
		case "card", "masked", "unmasked":
			if p.IsMasked {
				t.Errorf("%s: want unmasked", p.Key)
			}
		case ".pin":
			if !p.IsMasked {
				t.Errorf(".pin: want masked")
			}
		default:
			t.Errorf("unexpected key %q", p.Key)
		}
	}
}

func TestExtractKeyValuePairs_markdownSplit(t *testing.T) {
	input := "a: 1\n\n# Title\nbody\n"
	pairs, rest := ExtractKeyValuePairs(input)
	if len(pairs) != 1 || pairs[0].Key != "a" || pairs[0].Value != "1" {
		t.Fatalf("pairs: %+v", pairs)
	}
	wantRest := "# Title\nbody"
	if rest != wantRest {
		t.Fatalf("rest: got %q want %q", rest, wantRest)
	}
}

func TestExtractKeyValuePairs_nonKVLineStartsMarkdown(t *testing.T) {
	input := "k: v\nnot a kv line\nmore\n"
	pairs, rest := ExtractKeyValuePairs(input)
	if len(pairs) != 1 {
		t.Fatalf("pairs: %+v", pairs)
	}
	wantRest := "not a kv line\nmore"
	if rest != wantRest {
		t.Fatalf("rest: got %q want %q", rest, wantRest)
	}
}

func TestExtractKeyValuePairs_empty(t *testing.T) {
	pairs, rest := ExtractKeyValuePairs("")
	if pairs != nil || rest != "" {
		t.Fatalf("pairs=%v rest=%q", pairs, rest)
	}
}

func TestDisplayFieldName(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"email", "Email"},
		{".secret", "Secret"},
		{"e-mail", "E-Mail"},
		{"", ""},
		{".", ""},
	}
	for _, tt := range tests {
		if got := displayFieldName(tt.raw); got != tt.want {
			t.Errorf("displayFieldName(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}
