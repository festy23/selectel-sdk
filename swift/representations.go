package swift

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"mime"
	"strings"

	"github.com/festy23/selectel-sdk/types"
)

func listAccept(format ListFormat) string {
	if format == ListXML {
		return "application/xml"
	}
	if format == ListPlain {
		return "text/plain"
	}
	return "application/json"
}

// XML rows map to the same generated DTO fields as JSON. No XML entity resolver
// is installed, and document directives are rejected rather than interpreted.
func xmlListing(raw []byte, rootKind, owner string) ([]map[string]json.RawMessage, error) {
	scan := xml.NewDecoder(bytes.NewReader(raw))
	depth := 0
	for {
		tok, e := scan.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, types.NewError(types.ProtocolError)
		}
		switch tok.(type) {
		case xml.Directive:
			return nil, types.NewError(types.ProtocolError)
		case xml.StartElement:
			depth++
			if depth > 16 {
				return nil, types.NewError(types.ResponseLimit)
			}
		case xml.EndElement:
			depth--
		}
	}
	d := xml.NewDecoder(bytes.NewReader(raw))
	rows := []map[string]json.RawMessage{}
	var root xml.StartElement
	for {
		tok, e := d.Token()
		if e != nil {
			return nil, types.NewError(types.ProtocolError)
		}
		switch v := tok.(type) {
		case xml.ProcInst:
			if v.Target != "xml" {
				return nil, types.NewError(types.ProtocolError)
			}
		case xml.CharData:
			if strings.TrimSpace(string(v)) != "" {
				return nil, types.NewError(types.ProtocolError)
			}
		case xml.Comment:
		case xml.StartElement:
			root = v
			goto body
		default:
			return nil, types.NewError(types.ProtocolError)
		}
	}
body:
	if root.Name.Space != "" || root.Name.Local != rootKind {
		return nil, types.NewError(types.ProtocolError)
	}
	for _, a := range root.Attr {
		if a.Name.Local == "name" && a.Value != owner {
			return nil, types.NewError(types.ScopeMismatch)
		}
	}
	for {
		tok, e := d.Token()
		if e != nil {
			return nil, types.NewError(types.ProtocolError)
		}
		switch v := tok.(type) {
		case xml.CharData:
			if strings.TrimSpace(string(v)) != "" {
				return nil, types.NewError(types.ProtocolError)
			}
		case xml.Comment:
		case xml.EndElement:
			if v.Name != root.Name {
				return nil, types.NewError(types.ProtocolError)
			}
			for {
				t, e := d.Token()
				if e == io.EOF {
					return rows, nil
				}
				if e != nil {
					return nil, types.NewError(types.ProtocolError)
				}
				switch x := t.(type) {
				case xml.CharData:
					if strings.TrimSpace(string(x)) != "" {
						return nil, types.NewError(types.ProtocolError)
					}
				case xml.Comment:
				default:
					return nil, types.NewError(types.ProtocolError)
				}
			}
		case xml.StartElement:
			expected := "object"
			if rootKind == "account" {
				expected = "container"
			}
			if v.Name.Space != "" || (v.Name.Local != expected && v.Name.Local != "subdir") {
				return nil, types.NewError(types.ProtocolError)
			}
			var wire struct {
				Name         *string `xml:"name"`
				Attribute    *string `xml:"name,attr"`
				Count        *string `xml:"count"`
				Bytes        *string `xml:"bytes"`
				Hash         *string `xml:"hash"`
				ContentType  *string `xml:"content_type"`
				LastModified *string `xml:"last_modified"`
			}
			if d.DecodeElement(&wire, &v) != nil {
				return nil, types.NewError(types.ProtocolError)
			}
			fields := map[string]json.RawMessage{}
			if v.Name.Local == "subdir" {
				name := wire.Name
				if name == nil {
					name = wire.Attribute
				}
				if name == nil || (wire.Name != nil && wire.Attribute != nil && *wire.Name != *wire.Attribute) {
					return nil, types.NewError(types.ProtocolError)
				}
				fields["subdir"], _ = json.Marshal(name)
			} else {
				for _, f := range []struct {
					k string
					v *string
				}{{"name", wire.Name}, {"hash", wire.Hash}, {"content_type", wire.ContentType}, {"last_modified", wire.LastModified}} {
					if f.v != nil {
						fields[f.k], _ = json.Marshal(*f.v)
					}
				}
				for _, f := range []struct {
					k string
					v *string
				}{{"count", wire.Count}, {"bytes", wire.Bytes}} {
					if f.v != nil {
						n, e := types.ParseInteger(*f.v)
						if e != nil {
							return nil, types.NewError(types.ProtocolError)
						}
						fields[f.k], _ = json.Marshal(n)
					}
				}
			}
			rows = append(rows, fields)
		default:
			return nil, types.NewError(types.ProtocolError)
		}
	}
}

func plainListing(raw []byte, ambiguous bool) ([]map[string]json.RawMessage, error) {
	rows := []map[string]json.RawMessage{}
	if len(raw) == 0 {
		return rows, nil
	}
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if !validName(line) {
			return nil, types.NewError(types.ProtocolError)
		}
		key := "name"
		if ambiguous {
			key = "listed_name"
		}
		encoded, _ := json.Marshal(line)
		rows = append(rows, map[string]json.RawMessage{key: encoded})
	}
	return rows, nil
}

func listContentType(format ListFormat, header string) bool {
	if header == "" {
		return true
	}
	kind, _, e := mime.ParseMediaType(header)
	if e != nil {
		return false
	}
	if format == ListXML {
		return kind == "application/xml" || kind == "text/xml"
	}
	return kind == listAccept(format)
}
