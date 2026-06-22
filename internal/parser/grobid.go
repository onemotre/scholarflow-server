package parser

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
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
	Monogr   teiMonogr   `xml:"monogr"`
	Idnos    []teiIdno   `xml:"idno"`
}

type teiMonogr struct {
	Title   string      `xml:"title"`
	Authors []teiAuthor `xml:"author"`
	Imprint teiImprint  `xml:"imprint"`
}

type teiImprint struct {
	Date teiDate `xml:"date"`
}

type teiDate struct {
	When string `xml:"when,attr"`
}

type teiIdno struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
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
	Back teiBack `xml:"back"`
}

type teiBack struct {
	Refs []teiRefBibl `xml:"div>listBibl>biblStruct"`
}

type teiRefBibl struct {
	Analytic teiRefAnalytic `xml:"analytic"`
	Monogr   teiMonogr      `xml:"monogr"`
	Idnos    []teiIdno      `xml:"idno"`
}

type teiRefAnalytic struct {
	Title   string      `xml:"title"`
	Authors []teiAuthor `xml:"author"`
}

type teiBody struct {
	Divs    []teiDiv    `xml:"div"`
	Figures []teiFigure `xml:"figure"`
}

type teiFigure struct {
	Type   string `xml:"type,attr"`
	Coords string `xml:"coords,attr"`
	Head   string `xml:"head"`
	Desc   string `xml:"figDesc"`
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
	bibl := doc.Header.FileDesc.SourceDesc.Bibl
	parsed.DOI = doiFromIdnos(bibl.Idnos)
	parsed.Year = parseYear(bibl.Monogr.Imprint.Date.When)
	for i, ref := range doc.Text.Back.Refs {
		title := strings.TrimSpace(ref.Analytic.Title)
		venue := strings.TrimSpace(ref.Monogr.Title)
		if title == "" {
			title, venue = venue, ""
		}
		authors := authorNames(ref.Analytic.Authors)
		if len(authors) == 0 {
			authors = authorNames(ref.Monogr.Authors)
		}
		parsed.References = append(parsed.References, Reference{
			Order:   int32(i + 1),
			Title:   title,
			Authors: authors,
			Venue:   venue,
			Year:    parseYear(ref.Monogr.Imprint.Date.When),
			DOI:     doiFromIdnos(ref.Idnos),
		})
	}
	for i, fig := range doc.Text.Body.Figures {
		kind := "figure"
		fallback := "Figure"
		if strings.EqualFold(strings.TrimSpace(fig.Type), "table") {
			kind = "table"
			fallback = "Table"
		}
		label := strings.TrimSpace(fig.Head)
		if label == "" {
			label = fmt.Sprintf("%s %d", fallback, i+1)
		}
		parsed.Figures = append(parsed.Figures, Figure{
			Order:   int32(i + 1),
			Kind:    kind,
			Label:   label,
			Caption: strings.TrimSpace(fig.Desc),
			Page:    parsePage(fig.Coords),
		})
	}
	return parsed, nil
}

func doiFromIdnos(idnos []teiIdno) string {
	for _, id := range idnos {
		if strings.EqualFold(strings.TrimSpace(id.Type), "DOI") {
			return strings.TrimSpace(id.Value)
		}
	}
	return ""
}

func parseYear(when string) int32 {
	when = strings.TrimSpace(when)
	if len(when) < 4 {
		return 0
	}
	year, err := strconv.Atoi(when[:4])
	if err != nil {
		return 0
	}
	return int32(year)
}

func authorNames(authors []teiAuthor) []string {
	var names []string
	for _, a := range authors {
		name := strings.TrimSpace(a.PersName.Forename + " " + a.PersName.Surname)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func parsePage(coords string) *int32 {
	coords = strings.TrimSpace(coords)
	if coords == "" {
		return nil
	}
	first := coords
	if idx := strings.IndexByte(coords, ','); idx >= 0 {
		first = coords[:idx]
	}
	page, err := strconv.Atoi(strings.TrimSpace(first))
	if err != nil {
		return nil
	}
	value := int32(page)
	return &value
}
