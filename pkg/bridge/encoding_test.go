package bridge

import (
	"bytes"
	"io"
	"testing"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

func TestNewDecodingReader_UTF8Passthrough(t *testing.T) {
	input := "hello world"
	r, err := NewDecodingReader(bytes.NewReader([]byte(input)), "utf8")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	if string(got) != input {
		t.Errorf("got %q, want %q", got, input)
	}
}

func TestNewDecodingReader_EmptyEncoding(t *testing.T) {
	input := "passthrough"
	r, err := NewDecodingReader(bytes.NewReader([]byte(input)), "")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	if string(got) != input {
		t.Errorf("got %q, want %q", got, input)
	}
}

func TestNewDecodingReader_CP1252(t *testing.T) {
	// Encode "café" in CP1252: 'c' 'a' 'f' 0xe9
	cp1252Bytes := []byte{0x63, 0x61, 0x66, 0xe9}
	r, err := NewDecodingReader(bytes.NewReader(cp1252Bytes), "cp1252")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	want := "café"
	if string(got) != want {
		t.Errorf("CP1252 decode: got %q, want %q", got, want)
	}
}

func TestNewDecodingReader_UTF16LE(t *testing.T) {
	// "Hi" in UTF-16LE: 'H'=0x48,0x00  'i'=0x69,0x00
	utf16le := []byte{0x48, 0x00, 0x69, 0x00}
	r, err := NewDecodingReader(bytes.NewReader(utf16le), "utf16le")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	if string(got) != "Hi" {
		t.Errorf("UTF-16LE decode: got %q, want %q", got, "Hi")
	}
}

func TestNewDecodingReader_UTF16BE(t *testing.T) {
	// "Hi" in UTF-16BE: 'H'=0x00,0x48  'i'=0x00,0x69
	utf16be := []byte{0x00, 0x48, 0x00, 0x69}
	r, err := NewDecodingReader(bytes.NewReader(utf16be), "utf16be")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	if string(got) != "Hi" {
		t.Errorf("UTF-16BE decode: got %q, want %q", got, "Hi")
	}
}

func TestNewDecodingReader_AutoBOM_UTF16LE(t *testing.T) {
	// UTF-16LE BOM (FF FE) + "A" (0x41 0x00)
	data := []byte{0xFF, 0xFE, 0x41, 0x00}
	r, err := NewDecodingReader(bytes.NewReader(data), "auto")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	// Decoder should produce "A" (BOM may or may not be stripped, but content should decode).
	if !bytes.Contains(got, []byte("A")) {
		t.Errorf("Auto BOM UTF-16LE: got %q, expected it to contain 'A'", got)
	}
}

func TestNewDecodingReader_AutoNoBOM(t *testing.T) {
	input := "plain text"
	r, err := NewDecodingReader(bytes.NewReader([]byte(input)), "auto")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(r)
	if string(got) != input {
		t.Errorf("Auto no BOM: got %q, want %q", got, input)
	}
}

func TestNewDecodingReader_UnsupportedEncoding(t *testing.T) {
	_, err := NewDecodingReader(bytes.NewReader(nil), "ebcdic")
	if err == nil {
		t.Error("expected error for unsupported encoding")
	}
}

func TestResolveEncoding_Aliases(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"windows-1252", "cp1252"},
		{"latin1", "cp1252"},
		{"iso-8859-1", "cp1252"},
		{"utf-16le", "utf16le"},
		{"utf-16be", "utf16be"},
		{"utf-8", "utf8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := resolveEncoding(tt.name)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			switch tt.want {
			case "utf8":
				if enc != nil {
					t.Error("expected nil for UTF-8")
				}
			case "cp1252":
				if enc != charmap.Windows1252 {
					t.Error("expected Windows1252")
				}
			case "utf16le":
				if enc == nil {
					t.Error("expected non-nil for UTF-16LE")
				}
			case "utf16be":
				if enc == nil {
					t.Error("expected non-nil for UTF-16BE")
				}
			}
		})
	}
}

// Suppress unused import warnings.
var _ = unicode.UTF16
