package parser

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

type GROBIDParser struct {
	baseURL string
	client  *http.Client
}

func NewGROBIDParser(baseURL string) *GROBIDParser {
	return &GROBIDParser{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 2 * time.Minute},
	}
}

func (p *GROBIDParser) ParsePDF(ctx context.Context, filename string, body io.Reader) (ParsedPaper, error) {
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)
	part, err := writer.CreateFormFile("input", filename)
	if err != nil {
		return ParsedPaper{}, err
	}
	if _, err := io.Copy(part, body); err != nil {
		return ParsedPaper{}, err
	}
	if err := writer.Close(); err != nil {
		return ParsedPaper{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/processFulltextDocument", &payload)
	if err != nil {
		return ParsedPaper{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.client.Do(req)
	if err != nil {
		return ParsedPaper{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return ParsedPaper{}, fmt.Errorf("grobid returned status %d", resp.StatusCode)
	}
	tei, err := io.ReadAll(resp.Body)
	if err != nil {
		return ParsedPaper{}, err
	}
	return parseTEI(tei)
}

type teiDoc struct {
	Header teiHeader `xml:"teiHeader"`
	Text   teiText   `xml:"text"`
}

type teiHeader struct {
	FileDesc    teiFileDesc    `xml:"fileDesc"`
	ProfileDesc teiProfileDesc `xml:"profileDesc"`
}

type teiFileDesc struct {
	TitleStmt  teiTitleStmt  `xml:"titleStmt"`
	SourceDesc teiSourceDesc `xml:"sourceDesc"`
}

type teiTitleStmt struct {
	Title string `xml:"title"`
}

type teiSourceDesc struct {
	Bibl teiBiblStruct `xml:"biblStruct"`
}

type teiBiblStruct struct {
	Analytic teiAnalytic `xml:"analytic"`
}

type teiAnalytic struct {
	Authors []teiAuthor `xml:"author"`
}

type teiAuthor struct {
	PersName teiPersName `xml:"persName"`
}

type teiPersName struct {
	Forename string `xml:"forename"`
	Surname  string `xml:"surname"`
}

type teiProfileDesc struct {
	Abstract teiParagraphs `xml:"abstract"`
}

type teiText struct {
	Body teiBody `xml:"body"`
}

type teiBody struct {
	Divs []teiDiv `xml:"div"`
}

type teiDiv struct {
	Head       string   `xml:"head"`
	Paragraphs []string `xml:"p"`
}

type teiParagraphs struct {
	Paragraphs []string `xml:"p"`
}

func parseTEI(raw []byte) (ParsedPaper, error) {
	var doc teiDoc
	if err := xml.Unmarshal(raw, &doc); err != nil {
		return ParsedPaper{}, err
	}
	parsed := ParsedPaper{
		Title:    strings.TrimSpace(doc.Header.FileDesc.TitleStmt.Title),
		Abstract: strings.TrimSpace(strings.Join(doc.Header.ProfileDesc.Abstract.Paragraphs, "\n\n")),
		RawTEI:   string(raw),
	}
	for i, author := range doc.Header.FileDesc.SourceDesc.Bibl.Analytic.Authors {
		name := strings.TrimSpace(strings.Join([]string{author.PersName.Forename, author.PersName.Surname}, " "))
		if name == "" {
			continue
		}
		parsed.Authors = append(parsed.Authors, Author{Order: int32(i + 1), DisplayName: name})
	}
	for i, div := range doc.Text.Body.Divs {
		text := strings.TrimSpace(strings.Join(div.Paragraphs, "\n\n"))
		if text == "" {
			continue
		}
		parsed.Sections = append(parsed.Sections, Section{
			Order:   int32(i + 1),
			Heading: strings.TrimSpace(div.Head),
			Text:    text,
		})
	}
	return parsed, nil
}
