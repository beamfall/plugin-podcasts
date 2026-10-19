package podcasts

import (
	"encoding/xml"
	"errors"
	"io"
	"net/url"
	"strings"
)

// maxOPMLBytes caps the input read before XML decoding to prevent
// memory-exhaustion DoS from a large/deeply-nested OPML document.
const (
	maxOPMLBytes    = 64 << 20 // 64 MB
	maxOPMLDepth    = 64
	maxOPMLOutlines = 10_000
)

var (
	ErrOPMLTooLarge        = errors.New("podcasts: OPML exceeds byte limit")
	ErrOPMLTooDeep         = errors.New("podcasts: OPML exceeds nesting limit")
	ErrOPMLTooManyOutlines = errors.New("podcasts: OPML exceeds outline limit")
)

type opmlDocument struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    opmlHead `xml:"head"`
	Body    opmlBody `xml:"body"`
}

func (doc *opmlDocument) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	if start.Name.Local != "opml" {
		return errors.New("podcasts: XML root is not OPML")
	}
	doc.XMLName = start.Name
	for _, attr := range start.Attr {
		if attr.Name.Local == "version" {
			doc.Version = attr.Value
		}
	}
	type outlineNode struct {
		outline  opmlOutline
		children []*outlineNode
	}
	attrs := func(element xml.StartElement) opmlOutline {
		var outline opmlOutline
		for _, attr := range element.Attr {
			switch attr.Name.Local {
			case "text":
				outline.Text = attr.Value
			case "title":
				outline.Title = attr.Value
			case "xmlUrl":
				outline.XMLURL = attr.Value
			case "htmlUrl":
				outline.HTMLURL = attr.Value
			case "guid":
				outline.GUID = attr.Value
			case "played":
				outline.Played = attr.Value
			case "progress":
				outline.Progress = attr.Value
			case "starred":
				outline.Starred = attr.Value
			}
		}
		return outline
	}

	depth, outlineCount, bodyDepth := 1, 0, 0
	elements := []string{start.Name.Local}
	var roots []*outlineNode
	var outlineStack []*outlineNode
	var title strings.Builder
	for depth > 0 {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		switch token := token.(type) {
		case xml.StartElement:
			depth++
			if depth > maxOPMLDepth {
				return ErrOPMLTooDeep
			}
			name := token.Name.Local
			elements = append(elements, name)
			if name == "body" && bodyDepth == 0 {
				bodyDepth = depth
			}
			if name == "outline" && bodyDepth > 0 {
				outlineCount++
				if outlineCount > maxOPMLOutlines {
					return ErrOPMLTooManyOutlines
				}
				node := &outlineNode{outline: attrs(token)}
				if len(outlineStack) == 0 {
					roots = append(roots, node)
				} else {
					parent := outlineStack[len(outlineStack)-1]
					parent.children = append(parent.children, node)
				}
				outlineStack = append(outlineStack, node)
			}
		case xml.CharData:
			if len(elements) >= 2 && elements[len(elements)-1] == "title" && elements[len(elements)-2] == "head" {
				title.Write(token)
			}
		case xml.EndElement:
			if token.Name.Local == "outline" && len(outlineStack) > 0 {
				outlineStack = outlineStack[:len(outlineStack)-1]
			}
			if token.Name.Local == "body" && bodyDepth == depth {
				bodyDepth = 0
			}
			if len(elements) > 0 {
				elements = elements[:len(elements)-1]
			}
			depth--
		}
	}
	var materialize func(*outlineNode) opmlOutline
	materialize = func(node *outlineNode) opmlOutline {
		outline := node.outline
		if len(node.children) > 0 {
			outline.Outlines = make([]opmlOutline, 0, len(node.children))
			for _, child := range node.children {
				outline.Outlines = append(outline.Outlines, materialize(child))
			}
		}
		return outline
	}
	doc.Head.Title = title.String()
	doc.Body.Outlines = make([]opmlOutline, 0, len(roots))
	for _, root := range roots {
		doc.Body.Outlines = append(doc.Body.Outlines, materialize(root))
	}
	return nil
}

type opmlHead struct {
	Title string `xml:"title"`
}

type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

type opmlOutline struct {
	Text     string        `xml:"text,attr,omitempty"`
	Title    string        `xml:"title,attr,omitempty"`
	XMLURL   string        `xml:"xmlUrl,attr,omitempty"`
	HTMLURL  string        `xml:"htmlUrl,attr,omitempty"`
	GUID     string        `xml:"guid,attr,omitempty"`
	Played   string        `xml:"played,attr,omitempty"`
	Progress string        `xml:"progress,attr,omitempty"`
	Starred  string        `xml:"starred,attr,omitempty"`
	Outlines []opmlOutline `xml:"outline,omitempty"`
}

// ParseOPML reads a subscription tree, preserving nested folders and feed URLs.
func ParseOPML(r io.Reader) (Document, error) {
	raw, err := decodeOPMLDocument(r)
	if err != nil {
		return Document{}, err
	}
	doc := Document{Title: strings.TrimSpace(raw.Head.Title)}
	doc.Outlines = make([]Outline, 0, len(raw.Body.Outlines))
	for _, outline := range raw.Body.Outlines {
		parsed, err := convertOutline(outline)
		if err != nil {
			return Document{}, err
		}
		doc.Outlines = append(doc.Outlines, parsed)
	}
	return doc, nil
}

func decodeOPMLDocument(r io.Reader) (opmlDocument, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxOPMLBytes+1))
	if err != nil {
		return opmlDocument{}, err
	}
	if len(data) > maxOPMLBytes {
		return opmlDocument{}, ErrOPMLTooLarge
	}
	var raw opmlDocument
	if err := xml.Unmarshal(data, &raw); err != nil {
		return opmlDocument{}, err
	}
	return raw, nil
}

func convertOutline(in opmlOutline) (Outline, error) {
	out := Outline{
		Text:    strings.TrimSpace(in.Text),
		Title:   strings.TrimSpace(in.Title),
		XMLURL:  strings.TrimSpace(in.XMLURL),
		HTMLURL: strings.TrimSpace(in.HTMLURL),
	}
	if out.Text == "" {
		out.Text = out.Title
	}
	if out.Title == "" {
		out.Title = out.Text
	}
	if out.XMLURL != "" {
		parsed, err := readSafeFeedURL(out.XMLURL)
		if err != nil {
			return Outline{}, err
		}
		out.XMLURL = parsed
	}
	if len(in.Outlines) > 0 {
		out.Children = make([]Outline, 0, len(in.Outlines))
		for _, child := range in.Outlines {
			parsed, err := convertOutline(child)
			if err != nil {
				return Outline{}, err
			}
			out.Children = append(out.Children, parsed)
		}
	}
	return out, nil
}

// WriteOPML emits a well-formed OPML 2.0 document.
func WriteOPML(w io.Writer, doc Document) error {
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	root := opmlDocument{
		Version: "2.0",
		Head:    opmlHead{Title: strings.TrimSpace(doc.Title)},
		Body:    opmlBody{Outlines: make([]opmlOutline, 0, len(doc.Outlines))},
	}
	for _, outline := range doc.Outlines {
		converted := toOPMLOutline(outline)
		root.Body.Outlines = append(root.Body.Outlines, converted)
	}
	if err := enc.Encode(root); err != nil {
		return err
	}
	return enc.Flush()
}

func toOPMLOutline(in Outline) opmlOutline {
	out := opmlOutline{
		Text:    strings.TrimSpace(in.Text),
		Title:   strings.TrimSpace(in.Title),
		XMLURL:  strings.TrimSpace(in.XMLURL),
		HTMLURL: strings.TrimSpace(in.HTMLURL),
	}
	if out.Text == "" {
		out.Text = out.Title
	}
	if out.Title == "" {
		out.Title = out.Text
	}
	if len(in.Children) == 0 {
		return out
	}
	out.Outlines = make([]opmlOutline, 0, len(in.Children))
	for _, child := range in.Children {
		out.Outlines = append(out.Outlines, toOPMLOutline(child))
	}
	return out
}

// readSafeFeedURL validates an XML URL attribute without leaking the raw value
// into returned errors. Surrounding whitespace is trimmed before validation:
// the OPML callers pre-trim, but the directory-search callers (parseFeeds,
// parseITunesResults) pass raw JSON values through, and a stray newline must
// not reject an otherwise safe https feed URL.
func readSafeFeedURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", invalidFeedURLError()
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", invalidFeedURLError()
	}
	if parsed.User != nil {
		return "", invalidFeedURLError()
	}
	return parsed.String(), nil
}
