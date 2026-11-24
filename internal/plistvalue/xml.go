package plistvalue

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"maps"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// dateLayout is the XML plist date format: ISO 8601 in UTC, whole seconds.
const dateLayout = "2006-01-02T15:04:05Z"

const documentHeader = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
`

// DecodeDocument parses a complete XML property list, as printed by
// `defaults export <domain> -`, and returns its root value.
func DecodeDocument(doc []byte) (Value, error) {
	d := xml.NewDecoder(bytes.NewReader(doc))
	for {
		tok, err := d.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return Value{}, errors.New("plist: no <plist> element")
			}
			return Value{}, fmt.Errorf("plist: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "plist" {
			return Value{}, fmt.Errorf("plist: root element is <%s>, want <plist>", start.Name.Local)
		}
		break
	}

	var root *Value
	for {
		tok, err := d.Token()
		if err != nil {
			return Value{}, fmt.Errorf("plist: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if root != nil {
				return Value{}, errors.New("plist: more than one value inside <plist>")
			}
			v, err := decodeValue(d, t)
			if err != nil {
				return Value{}, err
			}
			root = &v
		case xml.EndElement:
			if root == nil {
				return Value{}, errors.New("plist: empty <plist>")
			}
			return *root, nil
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return Value{}, errors.New("plist: unexpected text inside <plist>")
			}
		}
	}
}

// DecodeDomain parses a `defaults export` document and returns its keys. The
// root must be a dict.
func DecodeDomain(doc []byte) (map[string]Value, error) {
	root, err := DecodeDocument(doc)
	if err != nil {
		return nil, err
	}
	if root.Kind != KindDict {
		return nil, fmt.Errorf("plist: domain root is %s, want dict", root.Kind)
	}
	return root.Dict, nil
}

func decodeValue(d *xml.Decoder, start xml.StartElement) (Value, error) {
	name := start.Name.Local
	switch name {
	case "true", "false":
		if err := d.Skip(); err != nil {
			return Value{}, fmt.Errorf("plist: <%s>: %w", name, err)
		}
		return Boolean(name == "true"), nil
	case "string":
		s, err := readText(d, name)
		return String(s), err
	case "integer":
		s, err := readText(d, name)
		if err != nil {
			return Value{}, err
		}
		i, ok := new(big.Int).SetString(strings.TrimSpace(s), 10)
		if !ok {
			return Value{}, fmt.Errorf("plist: invalid <integer> %q", s)
		}
		return Value{Kind: KindInteger, Int: i}, nil
	case "real":
		s, err := readText(d, name)
		if err != nil {
			return Value{}, err
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return Value{}, fmt.Errorf("plist: invalid <real> %q", s)
		}
		return Float(f), nil
	case "date":
		s, err := readText(d, name)
		if err != nil {
			return Value{}, err
		}
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
		if err != nil {
			return Value{}, fmt.Errorf("plist: invalid <date> %q", s)
		}
		return Date(t), nil
	case "data":
		s, err := readText(d, name)
		if err != nil {
			return Value{}, err
		}
		b, err := base64.StdEncoding.DecodeString(stripSpace(s))
		if err != nil {
			return Value{}, fmt.Errorf("plist: invalid <data>: %w", err)
		}
		return Data(b), nil
	case "array":
		return decodeArray(d)
	case "dict":
		return decodeDict(d)
	default:
		return Value{}, fmt.Errorf("plist: unknown element <%s>", name)
	}
}

func decodeArray(d *xml.Decoder) (Value, error) {
	elems := []Value{}
	for {
		tok, err := d.Token()
		if err != nil {
			return Value{}, fmt.Errorf("plist: <array>: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			v, err := decodeValue(d, t)
			if err != nil {
				return Value{}, err
			}
			elems = append(elems, v)
		case xml.EndElement:
			return Array(elems...), nil
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return Value{}, errors.New("plist: unexpected text in <array>")
			}
		}
	}
}

func decodeDict(d *xml.Decoder) (Value, error) {
	m := map[string]Value{}
	var key *string
	for {
		tok, err := d.Token()
		if err != nil {
			return Value{}, fmt.Errorf("plist: <dict>: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if key == nil {
				if t.Name.Local != "key" {
					return Value{}, fmt.Errorf("plist: <dict> has <%s> where a <key> belongs", t.Name.Local)
				}
				k, err := readText(d, "key")
				if err != nil {
					return Value{}, err
				}
				key = &k
				continue
			}
			v, err := decodeValue(d, t)
			if err != nil {
				return Value{}, fmt.Errorf("%w (in key %q)", err, *key)
			}
			m[*key] = v
			key = nil
		case xml.EndElement:
			if key != nil {
				return Value{}, fmt.Errorf("plist: <dict> key %q has no value", *key)
			}
			return Dict(m), nil
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return Value{}, errors.New("plist: unexpected text in <dict>")
			}
		}
	}
}

// readText reads character data up to the closing tag of name.
func readText(d *xml.Decoder, name string) (string, error) {
	var b strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			return "", fmt.Errorf("plist: <%s>: %w", name, err)
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			return b.String(), nil
		case xml.StartElement:
			return "", fmt.Errorf("plist: unexpected <%s> inside <%s>", t.Name.Local, name)
		}
	}
}

func stripSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

// EncodeXML renders v as the compact XML fragment `defaults write` accepts as
// a value argument, e.g. `<dict><key>a</key><true/></dict>`.
func EncodeXML(v Value) (string, error) {
	var b strings.Builder
	if err := encode(&b, v); err != nil {
		return "", err
	}
	return b.String(), nil
}

// EncodeDocument renders a complete XML property list document with root v,
// in the same shape `defaults export` prints.
func EncodeDocument(v Value) ([]byte, error) {
	frag, err := EncodeXML(v)
	if err != nil {
		return nil, err
	}
	return []byte(documentHeader + frag + "\n</plist>\n"), nil
}

func encode(b *strings.Builder, v Value) error {
	switch v.Kind {
	case KindString:
		return encodeText(b, "string", v.Str)
	case KindInteger:
		if v.Int == nil {
			return errors.New("plist: integer value has no number")
		}
		b.WriteString("<integer>")
		b.WriteString(v.Int.String())
		b.WriteString("</integer>")
	case KindFloat:
		b.WriteString("<real>")
		b.WriteString(strconv.FormatFloat(v.Float, 'g', -1, 64))
		b.WriteString("</real>")
	case KindBoolean:
		if v.Bool {
			b.WriteString("<true/>")
		} else {
			b.WriteString("<false/>")
		}
	case KindDate:
		b.WriteString("<date>")
		b.WriteString(v.Date.UTC().Format(dateLayout))
		b.WriteString("</date>")
	case KindData:
		b.WriteString("<data>")
		b.WriteString(base64.StdEncoding.EncodeToString(v.Data))
		b.WriteString("</data>")
	case KindArray:
		b.WriteString("<array>")
		for _, e := range v.Array {
			if err := encode(b, e); err != nil {
				return err
			}
		}
		b.WriteString("</array>")
	case KindDict:
		b.WriteString("<dict>")
		for _, k := range slices.Sorted(maps.Keys(v.Dict)) {
			if err := encodeText(b, "key", k); err != nil {
				return err
			}
			if err := encode(b, v.Dict[k]); err != nil {
				return err
			}
		}
		b.WriteString("</dict>")
	default:
		return fmt.Errorf("plist: unknown kind %q", v.Kind)
	}
	return nil
}

func encodeText(b *strings.Builder, tag, s string) error {
	if err := CheckXMLText(s); err != nil {
		return err
	}
	b.WriteString("<")
	b.WriteString(tag)
	b.WriteString(">")
	if err := xml.EscapeText(b, []byte(s)); err != nil {
		return fmt.Errorf("plist: escaping <%s>: %w", tag, err)
	}
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteString(">")
	return nil
}

// CheckXMLText reports an error if s can't be stored in an XML plist: it must
// be valid UTF-8 and contain only characters XML 1.0 allows (no control
// characters other than tab, newline and carriage return).
func CheckXMLText(s string) error {
	if !utf8.ValidString(s) {
		return errors.New("text is not valid UTF-8")
	}
	for i, r := range s {
		if !isXMLChar(r) {
			return fmt.Errorf("text contains %U at byte %d, which a property list can't store", r, i)
		}
	}
	return nil
}

func isXMLChar(r rune) bool {
	return r == 0x09 || r == 0x0A || r == 0x0D ||
		r >= 0x20 && r <= 0xD7FF ||
		r >= 0xE000 && r <= 0xFFFD ||
		r >= 0x10000 && r <= 0x10FFFF
}
