package probe

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

const maxProbePages = 10000

type Page[T any] struct {
	Items      []T
	TotalCount *int
}

type PageFetcher[T any] func(context.Context, int, int) (Page[T], error)

type ExactMatch[T any] struct {
	Count int
	Item  T
	Items []T
}

// CollectAllPages visits every required page and returns every item. It rejects
// inconsistent pagination metadata and bounds the number of requests.
func CollectAllPages[T any](
	ctx context.Context,
	pageSize int,
	fetch PageFetcher[T],
) ([]T, error) {
	if pageSize <= 0 {
		return nil, errors.New("page size must be positive")
	}
	if fetch == nil {
		return nil, errors.New("page fetcher is required")
	}
	items := []T{}
	seen := 0
	for pageIndex := 0; pageIndex < maxProbePages; pageIndex++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		page, err := fetch(ctx, pageIndex, pageSize)
		if err != nil {
			return nil, fmt.Errorf("fetch page %d: %w", pageIndex, err)
		}
		items = append(items, page.Items...)
		seen += len(page.Items)
		if page.TotalCount != nil {
			if *page.TotalCount < seen {
				return nil, errors.New(
					"page total count is smaller than items observed",
				)
			}
			if seen >= *page.TotalCount {
				return items, nil
			}
		} else if len(page.Items) < pageSize {
			return items, nil
		}
		if len(page.Items) == 0 {
			return items, nil
		}
	}
	return nil, errors.New("pagination exceeded safety limit")
}

// FindExactAcrossAllPages never adopts the first fuzzy or partial match. It
// visits every required page and counts only normalized exact equality.
func FindExactAcrossAllPages[T any](
	ctx context.Context,
	pageSize int,
	fetch PageFetcher[T],
	identity func(T) string,
	expected string,
	normalize func(string) string,
) (ExactMatch[T], error) {
	var result ExactMatch[T]
	if pageSize <= 0 {
		return result, errors.New("page size must be positive")
	}
	if fetch == nil || identity == nil {
		return result, errors.New("page fetcher and identity function are required")
	}
	if normalize == nil {
		normalize = func(value string) string { return value }
	}
	expected = normalize(expected)
	seen := 0
	for pageIndex := 0; pageIndex < maxProbePages; pageIndex++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		page, err := fetch(ctx, pageIndex, pageSize)
		if err != nil {
			return result, fmt.Errorf("fetch page %d: %w", pageIndex, err)
		}
		for _, item := range page.Items {
			if normalize(identity(item)) == expected {
				result.Count++
				result.Items = append(result.Items, item)
				if result.Count == 1 {
					result.Item = item
				}
			}
		}
		seen += len(page.Items)
		if page.TotalCount != nil {
			if *page.TotalCount < seen {
				return result, errors.New("page total count is smaller than items observed")
			}
			if seen >= *page.TotalCount {
				return result, nil
			}
		} else if len(page.Items) < pageSize {
			return result, nil
		}
		if len(page.Items) == 0 {
			return result, nil
		}
	}
	return result, errors.New("pagination exceeded safety limit")
}

func NormalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
