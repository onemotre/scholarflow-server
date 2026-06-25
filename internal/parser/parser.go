package parser

import (
	"context"
	"io"
)

type ParsedPaper struct {
	Title        string
	Abstract     string
	DOI          string
	Year         int32
	Authors      []Author
	Affiliations []Affiliation
	Sections     []Section
	References   []Reference
	Figures      []Figure
	RawTEI       string
}

type Author struct {
	Order       int32
	DisplayName string
	ORCID       string
}

type Affiliation struct {
	Raw        string
	Role       string
	Source     string
	Confidence float64
}

type Section struct {
	Order     int32
	Heading   string
	Text      string
	PageStart *int32
	PageEnd   *int32
	Anchor    string
}

type Reference struct {
	Order   int32
	Title   string
	Authors []string
	Venue   string
	Year    int32
	DOI     string
	RawText string
}

type FigureBox struct {
	Page       int32
	X, Y, W, H float64
}

type Figure struct {
	Order   int32
	Kind    string
	Label   string
	Caption string
	Page    *int32
	BBox    *FigureBox // nil when GROBID emitted no usable coords
}

type Parser interface {
	ParsePDF(ctx context.Context, filename string, body io.Reader) (ParsedPaper, error)
}
