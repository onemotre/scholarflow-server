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
