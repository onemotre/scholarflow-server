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

// coordinateElements are the TEI element names we ask GROBID to annotate with
// @coords (page,x,y,w,h), so figure and section pages can be recovered.
var coordinateElements = []string{"figure", "head", "p", "s"}

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
	// Ask GROBID to emit @coords on these elements so we can recover the page a
	// figure or section sits on. Without this, coords are absent and pages are nil.
	for _, tag := range coordinateElements {
		if err := writer.WriteField("teiCoordinates", tag); err != nil {
			return ParsedPaper{}, err
		}
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
	Head       teiTextCoord   `xml:"head"`
	Paragraphs []teiTextCoord `xml:"p"`
}

// teiTextCoord captures an element's inner text plus its @coords attribute.
// Inner is raw inner XML (so nested <ref>/<s> text is preserved); plainText
// strips the tags. Coords is GROBID's "page,x,y,w,h" (possibly ";"-separated).
type teiTextCoord struct {
	Coords string `xml:"coords,attr"`
	Inner  string `xml:",innerxml"`
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
		var paras []string
		var pages []int32
		pages = appendCoordPages(pages, div.Head.Coords)
		for _, p := range div.Paragraphs {
			if t := plainText(p.Inner); t != "" {
				paras = append(paras, t)
			}
			pages = appendCoordPages(pages, p.Coords)
		}
		text := strings.TrimSpace(strings.Join(paras, "\n\n"))
		if text == "" {
			continue
		}
		start, end := pageRange(pages)
		parsed.Sections = append(parsed.Sections, Section{
			Order:     int32(i + 1),
			Heading:   strings.TrimSpace(plainText(div.Head.Inner)),
			Text:      text,
			PageStart: start,
			PageEnd:   end,
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
			BBox:    parseBox(fig.Coords),
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

// plainText extracts the character data from a fragment of inner XML, dropping
// tags (e.g. nested <ref>/<s>) while keeping their text and resolving entities.
func plainText(inner string) string {
	if inner == "" {
		return ""
	}
	dec := xml.NewDecoder(strings.NewReader(inner))
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if cd, ok := tok.(xml.CharData); ok {
			b.Write(cd)
		}
	}
	return strings.TrimSpace(b.String())
}

// appendCoordPages parses every page index found in a GROBID @coords value
// ("page,x,y,w,h", possibly ";"-separated boxes) and appends them to pages.
func appendCoordPages(pages []int32, coords string) []int32 {
	coords = strings.TrimSpace(coords)
	if coords == "" {
		return pages
	}
	for _, box := range strings.Split(coords, ";") {
		box = strings.TrimSpace(box)
		if box == "" {
			continue
		}
		first := box
		if idx := strings.IndexByte(box, ','); idx >= 0 {
			first = box[:idx]
		}
		if p, err := strconv.Atoi(strings.TrimSpace(first)); err == nil && p > 0 {
			pages = append(pages, int32(p))
		}
	}
	return pages
}

// pageRange returns the min and max page in pages, or (nil, nil) when empty.
func pageRange(pages []int32) (*int32, *int32) {
	if len(pages) == 0 {
		return nil, nil
	}
	lo, hi := pages[0], pages[0]
	for _, p := range pages[1:] {
		if p < lo {
			lo = p
		}
		if p > hi {
			hi = p
		}
	}
	return &lo, &hi
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
	if err != nil || page <= 0 {
		return nil
	}
	value := int32(page)
	return &value
}

// parseBox unions the ";"-separated boxes in a GROBID @coords value into one
// rect on the dominant page (the page carrying the largest-area box). Returns
// nil when no box parses.
func parseBox(coords string) *FigureBox {
	coords = strings.TrimSpace(coords)
	if coords == "" {
		return nil
	}
	type box struct {
		page                 int32
		x0, y0, x1, y1, area float64
	}
	var boxes []box
	for _, raw := range strings.Split(coords, ";") {
		parts := strings.Split(strings.TrimSpace(raw), ",")
		if len(parts) < 5 {
			continue
		}
		page, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || page <= 0 {
			continue
		}
		x, errX := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		y, errY := strconv.ParseFloat(strings.TrimSpace(parts[2]), 64)
		wv, errW := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
		hv, errH := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64)
		if errX != nil || errY != nil || errW != nil || errH != nil {
			continue
		}
		boxes = append(boxes, box{page: int32(page), x0: x, y0: y, x1: x + wv, y1: y + hv, area: wv * hv})
	}
	if len(boxes) == 0 {
		return nil
	}
	dom := boxes[0]
	for _, b := range boxes[1:] {
		if b.area > dom.area {
			dom = b
		}
	}
	minX, minY, maxX, maxY := dom.x0, dom.y0, dom.x1, dom.y1
	for _, b := range boxes {
		if b.page != dom.page {
			continue
		}
		if b.x0 < minX {
			minX = b.x0
		}
		if b.y0 < minY {
			minY = b.y0
		}
		if b.x1 > maxX {
			maxX = b.x1
		}
		if b.y1 > maxY {
			maxY = b.y1
		}
	}
	return &FigureBox{Page: dom.page, X: minX, Y: minY, W: maxX - minX, H: maxY - minY}
}
