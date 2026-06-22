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
