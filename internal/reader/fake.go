package reader

import "context"

type FakeReader struct{}

func (FakeReader) ReadPaper(ctx context.Context, input Context) (PaperCard, error) {
	card := PaperCard{
		Background:     input.Abstract,
		Problem:        "not found",
		Method:         "not found",
		Implementation: "not found",
	}
	return card, card.Validate()
}
