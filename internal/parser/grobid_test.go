package parser

import "testing"

func TestParseTEIExtractsTitleAbstractAuthorAndSection(t *testing.T) {
	tei := `<?xml version="1.0"?>
<TEI>
  <teiHeader>
    <fileDesc>
      <titleStmt><title>Example Paper</title></titleStmt>
      <sourceDesc>
        <biblStruct>
          <analytic>
            <author><persName><forename>Ada</forename><surname>Lovelace</surname></persName></author>
          </analytic>
        </biblStruct>
      </sourceDesc>
    </fileDesc>
    <profileDesc>
      <abstract><p>This is the abstract.</p></abstract>
    </profileDesc>
  </teiHeader>
  <text>
    <body>
      <div><head>Introduction</head><p>This is the intro.</p></div>
    </body>
  </text>
</TEI>`

	parsed, err := parseTEI([]byte(tei))
	if err != nil {
		t.Fatalf("parseTEI returned error: %v", err)
	}
	if parsed.Title != "Example Paper" {
		t.Fatalf("Title = %q", parsed.Title)
	}
	if parsed.Abstract != "This is the abstract." {
		t.Fatalf("Abstract = %q", parsed.Abstract)
	}
	if len(parsed.Authors) != 1 || parsed.Authors[0].DisplayName != "Ada Lovelace" {
		t.Fatalf("Authors = %#v", parsed.Authors)
	}
	if len(parsed.Sections) != 1 || parsed.Sections[0].Heading != "Introduction" {
		t.Fatalf("Sections = %#v", parsed.Sections)
	}
}

func TestParseTEIExtractsDOIYearReferences(t *testing.T) {
	tei := `<TEI>
  <teiHeader><fileDesc>
    <titleStmt><title>T</title></titleStmt>
    <sourceDesc><biblStruct>
      <analytic><author><persName><forename>Ada</forename><surname>Lovelace</surname></persName></author></analytic>
      <monogr><imprint><date type="published" when="2022-07-19">2022</date></imprint></monogr>
      <idno type="arXiv">arXiv:1</idno>
      <idno type="DOI">10.1234/test</idno>
    </biblStruct></sourceDesc>
  </fileDesc></teiHeader>
  <text>
    <body><div><head>Intro</head><p>x</p></div></body>
    <back><div><listBibl>
      <biblStruct>
        <analytic><title level="a">Cited Paper</title>
          <author><persName><forename>Grace</forename><surname>Hopper</surname></persName></author></analytic>
        <monogr><title level="j">Journal of Tests</title><imprint><date when="1952">1952</date></imprint></monogr>
        <idno type="DOI">10.5555/cited</idno>
      </biblStruct>
    </listBibl></div></back>
  </text>
</TEI>`
	parsed, err := parseTEI([]byte(tei))
	if err != nil {
		t.Fatalf("parseTEI error: %v", err)
	}
	if parsed.DOI != "10.1234/test" {
		t.Fatalf("DOI = %q", parsed.DOI)
	}
	if parsed.Year != 2022 {
		t.Fatalf("Year = %d", parsed.Year)
	}
	if len(parsed.References) != 1 {
		t.Fatalf("References = %#v", parsed.References)
	}
	r := parsed.References[0]
	if r.Title != "Cited Paper" || r.Venue != "Journal of Tests" || r.Year != 1952 || r.DOI != "10.5555/cited" {
		t.Fatalf("Reference = %#v", r)
	}
	if len(r.Authors) != 1 || r.Authors[0] != "Grace Hopper" {
		t.Fatalf("Reference authors = %#v", r.Authors)
	}
}

func TestParseTEIExtractsFigures(t *testing.T) {
	tei := `<TEI><text><body>
	  <div><head>Intro</head><p>x</p></div>
	  <figure coords="3,10.0,20.0,30.0,40.0"><head>Figure 1</head><figDesc>A plot of results.</figDesc></figure>
	  <figure type="table"><figDesc>Table contents.</figDesc></figure>
	</body></text></TEI>`
	parsed, err := parseTEI([]byte(tei))
	if err != nil {
		t.Fatalf("parseTEI error: %v", err)
	}
	if len(parsed.Figures) != 2 {
		t.Fatalf("Figures = %#v", parsed.Figures)
	}
	f0 := parsed.Figures[0]
	if f0.Kind != "figure" || f0.Label != "Figure 1" || f0.Caption != "A plot of results." {
		t.Fatalf("Figure[0] = %#v", f0)
	}
	if f0.Page == nil || *f0.Page != 3 {
		t.Fatalf("Figure[0].Page = %v", f0.Page)
	}
	f1 := parsed.Figures[1]
	if f1.Kind != "table" || f1.Label != "Table 2" || f1.Caption != "Table contents." {
		t.Fatalf("Figure[1] = %#v", f1)
	}
	if f1.Page != nil {
		t.Fatalf("Figure[1].Page = %v, want nil", f1.Page)
	}
}
