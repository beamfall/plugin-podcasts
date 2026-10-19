package podcasts

import "fmt"

// Document is a generic OPML subscription tree.
type Document struct {
	Title    string
	Outlines []Outline
}

// Outline is one node in an OPML outline tree.
type Outline struct {
	Text     string
	Title    string
	XMLURL   string
	HTMLURL  string
	Children []Outline
}

// ItemState is the neutral import state keyed by feed URL + item GUID.
type ItemState struct {
	FeedURL     string
	GUID        string
	Played      bool
	Progress    float64
	HasProgress bool
	Starred     bool
}

func invalidFeedURLError() error {
	return fmt.Errorf("invalid feed URL")
}
