package probe

import (
	"context"
	"errors"
	"testing"
)

type identityFixture struct {
	ID    string
	Key   string
	Email string
}

func TestFindExactAcrossAllPagesDoesNotAdoptFuzzyFirstResult(t *testing.T) {
	t.Parallel()

	total := 5
	pages := [][]identityFixture{
		{{Key: "target-suffix"}, {Key: "another"}},
		{{Key: "target"}, {Key: "target-prefix"}},
		{{Key: "last"}},
	}
	fetches := 0
	match, err := FindExactAcrossAllPages(
		context.Background(),
		2,
		func(_ context.Context, pageIndex, _ int) (Page[identityFixture], error) {
			fetches++
			return Page[identityFixture]{Items: pages[pageIndex], TotalCount: &total}, nil
		},
		func(item identityFixture) string { return item.Key },
		"target",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if match.Count != 1 || match.Item.Key != "target" || fetches != 3 {
		t.Fatalf("match = %+v, fetches = %d", match, fetches)
	}
}

func TestFindExactAcrossAllPagesZeroOneAndDuplicateNormalizedEmail(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		pages [][]identityFixture
		want  int
	}{
		"zero": {
			pages: [][]identityFixture{{{Email: "other@example.test"}}},
			want:  0,
		},
		"one": {
			pages: [][]identityFixture{{{Email: " MEMBER@EXAMPLE.TEST "}}},
			want:  1,
		},
		"duplicate across pages": {
			pages: [][]identityFixture{
				{{Email: "member@example.test"}, {Email: "other@example.test"}},
				{{Email: " Member@Example.Test "}},
			},
			want: 2,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			match, err := FindExactAcrossAllPages(
				context.Background(),
				2,
				func(_ context.Context, pageIndex, _ int) (Page[identityFixture], error) {
					if pageIndex >= len(test.pages) {
						return Page[identityFixture]{}, nil
					}
					return Page[identityFixture]{Items: test.pages[pageIndex]}, nil
				},
				func(item identityFixture) string { return item.Email },
				"member@example.test",
				NormalizeEmail,
			)
			if err != nil {
				t.Fatal(err)
			}
			if match.Count != test.want || len(match.Items) != test.want {
				t.Fatalf(
					"match count/items = %d/%d, want %d",
					match.Count,
					len(match.Items),
					test.want,
				)
			}
		})
	}
}

func TestFindExactAcrossAllPagesPropagatesFailureAndCancellation(t *testing.T) {
	t.Parallel()

	expected := errors.New("page failure")
	_, err := FindExactAcrossAllPages(
		context.Background(),
		10,
		func(context.Context, int, int) (Page[identityFixture], error) {
			return Page[identityFixture]{}, expected
		},
		func(item identityFixture) string { return item.ID },
		"id",
		nil,
	)
	if !errors.Is(err, expected) {
		t.Fatalf("error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = FindExactAcrossAllPages(
		ctx,
		10,
		func(context.Context, int, int) (Page[identityFixture], error) {
			return Page[identityFixture]{}, nil
		},
		func(item identityFixture) string { return item.ID },
		"id",
		nil,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}
