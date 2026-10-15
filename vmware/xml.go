package vmware

import (
	"bytes"
	"encoding/xml"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"strings"
)

const versionsNamespace = "http://www.vmware.com/vcloud/versions"

// Source field names and namespaces are checked before identities/version flags
// are decoded. schemaLocation/login URLs are metadata, never follow-up targets.
func validateVersionsXML(raw []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	stack := []string{}
	counts := []map[string]int{}
	roots := 0
	allowed := map[string]map[string]bool{
		"":                  {"SupportedVersions": true},
		"SupportedVersions": {"VersionInfo": true, "AlphaVersion": true, "SchemaRoot": true},
		"VersionInfo":       {"Version": true, "LoginUrl": true, "ProviderLoginUrl": true, "MediaTypeMapping": true},
		"AlphaVersion":      {"Version": true, "LoginUrl": true, "ProviderLoginUrl": true, "MediaTypeMapping": true},
		"MediaTypeMapping":  {"MediaType": true, "ComplexTypeName": true, "SchemaLocation": true},
	}
	for {
		token, e := decoder.Token()
		if e == io.EOF {
			break
		}
		if e != nil {
			return types.NewError(types.ProtocolError)
		}
		switch value := token.(type) {
		case xml.StartElement:
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			if len(stack) >= 64 || value.Name.Space != versionsNamespace || !allowed[parent][value.Name.Local] {
				return types.NewError(types.ProtocolError)
			}
			if parent == "" {
				roots++
				if roots > 1 {
					return types.NewError(types.ProtocolError)
				}
			} else {
				counts[len(counts)-1][value.Name.Local]++
				repeated := parent == "SupportedVersions" && (value.Name.Local == "VersionInfo" || value.Name.Local == "AlphaVersion") || (parent == "VersionInfo" || parent == "AlphaVersion") && value.Name.Local == "MediaTypeMapping"
				if !repeated && counts[len(counts)-1][value.Name.Local] > 1 {
					return types.NewError(types.ProtocolError)
				}
			}
			attributes := map[xml.Name]bool{}
			for _, a := range value.Attr {
				if attributes[a.Name] {
					return types.NewError(types.ProtocolError)
				}
				attributes[a.Name] = true
				if a.Name.Space == "xmlns" || a.Name.Space == "" && a.Name.Local == "xmlns" {
					continue
				}
				if len(stack) == 0 && a.Name.Space == "http://www.w3.org/2001/XMLSchema-instance" && a.Name.Local == "schemaLocation" {
					continue
				}
				if (value.Name.Local == "VersionInfo" || value.Name.Local == "AlphaVersion") && a.Name.Space == "" && a.Name.Local == "deprecated" {
					continue
				}
				return types.NewError(types.ProtocolError)
			}
			stack = append(stack, value.Name.Local)
			counts = append(counts, map[string]int{})
		case xml.EndElement:
			if len(stack) == 0 {
				return types.NewError(types.ProtocolError)
			}
			stack = stack[:len(stack)-1]
			counts = counts[:len(counts)-1]
		case xml.ProcInst:
			if value.Target != "xml" {
				return types.NewError(types.ProtocolError)
			}
		case xml.Directive:
			return types.NewError(types.ProtocolError)
		case xml.CharData:
			if strings.TrimSpace(string(value)) != "" && (len(stack) == 0 || allowed[stack[len(stack)-1]] != nil) {
				return types.NewError(types.ProtocolError)
			}
		}
	}
	if roots != 1 || len(stack) != 0 {
		return types.NewError(types.ProtocolError)
	}
	return nil
}
