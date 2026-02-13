package bridge

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// Supported encoding names.
const (
	EncodingUTF8    = "utf8"
	EncodingCP1252  = "cp1252"
	EncodingUTF16LE = "utf16le"
	EncodingUTF16BE = "utf16be"
	EncodingAuto    = "auto"
)

// resolveEncoding maps a user-facing encoding name to a golang.org/x/text Encoding.
func resolveEncoding(name string) (encoding.Encoding, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case EncodingUTF8, "utf-8", "":
		return nil, nil // nil means passthrough (already UTF-8).
	case EncodingCP1252, "windows-1252", "latin1", "iso-8859-1":
		return charmap.Windows1252, nil
	case EncodingUTF16LE, "utf-16le":
		return unicode.UTF16(unicode.LittleEndian, unicode.IgnoreBOM), nil
	case EncodingUTF16BE, "utf-16be":
		return unicode.UTF16(unicode.BigEndian, unicode.IgnoreBOM), nil
	default:
		return nil, fmt.Errorf("unsupported encoding: %q (supported: utf8, cp1252, utf16le, utf16be, auto)", name)
	}
}

// detectBOMEncoding looks at the first bytes to detect a BOM and returns the
// appropriate encoding, plus the reader repositioned after the BOM.
func detectBOMEncoding(data []byte) encoding.Encoding {
	if len(data) >= 2 {
		// UTF-16 LE BOM: FF FE
		if data[0] == 0xFF && data[1] == 0xFE {
			return unicode.UTF16(unicode.LittleEndian, unicode.UseBOM)
		}
		// UTF-16 BE BOM: FE FF
		if data[0] == 0xFE && data[1] == 0xFF {
			return unicode.UTF16(unicode.BigEndian, unicode.UseBOM)
		}
	}
	if len(data) >= 3 {
		// UTF-8 BOM: EF BB BF — passthrough.
		if data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
			return nil
		}
	}
	return nil // No BOM detected, assume UTF-8.
}

// NewDecodingReader wraps an io.Reader to decode from the specified encoding to UTF-8.
//
// If enc is empty or "utf8", the reader is returned unmodified.
// If enc is "auto", BOM detection is attempted by peeking at the first bytes.
func NewDecodingReader(r io.Reader, enc string) (io.Reader, error) {
	if enc == "" || strings.ToLower(enc) == EncodingUTF8 {
		return r, nil
	}

	if strings.ToLower(enc) == EncodingAuto {
		return newAutoDetectReader(r)
	}

	e, err := resolveEncoding(enc)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return r, nil
	}
	return transform.NewReader(r, e.NewDecoder()), nil
}

// newAutoDetectReader peeks at the first bytes to detect encoding via BOM.
func newAutoDetectReader(r io.Reader) (io.Reader, error) {
	// Read enough bytes for BOM detection.
	buf := make([]byte, 4)
	n, err := io.ReadAtLeast(r, buf, 2)
	if err != nil && err != io.ErrUnexpectedEOF {
		if n == 0 {
			return r, nil
		}
	}
	peek := buf[:n]

	e := detectBOMEncoding(peek)
	// Reconstruct a reader with the peeked bytes prepended.
	combined := io.MultiReader(bytes.NewReader(peek), r)

	if e == nil {
		return combined, nil
	}
	return transform.NewReader(combined, e.NewDecoder()), nil
}
