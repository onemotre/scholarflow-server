package reader

import "context"

type FakeReader struct{}

func (FakeReader) ReadPaper(ctx context.Context, input Context) (PaperCard, error) {
	card := PaperCard{
		Introduction: input.Abstract,
		Methodology:  []MethodologyItem{{Problem: "not found", Method: "not found"}},
	}
	return card, card.Validate()
}
