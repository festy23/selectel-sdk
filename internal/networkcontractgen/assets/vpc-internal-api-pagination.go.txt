package api

import (
	"context"
	"fmt"
	"net/url"
)

// PageLink is one link in a paginated collection response.
type PageLink struct {
	Rel  string `json:"rel"`
	Href string `json:"href"`
}

// NextPageLink returns the URL of the next page, or an empty string on the last page.
func NextPageLink(links []PageLink) string {
	for _, link := range links {
		if link.Rel == "next" {
			return link.Href
		}
	}

	return ""
}

type Page[T any] struct {
	Items    []T
	NextLink string
}

func WalkPages[T any](
	ctx context.Context,
	initial url.Values,
	fetch func(context.Context, url.Values) (Page[T], error),
) ([]T, error) {
	query := cloneValues(initial)
	var result []T

	for {
		page, err := fetch(ctx, query)
		if err != nil {
			return nil, err
		}
		result = append(result, page.Items...)

		if page.NextLink == "" {
			return result, nil
		}

		nextURL, err := url.Parse(page.NextLink)
		if err != nil {
			return nil, fmt.Errorf("parse next page link: %w", err)
		}
		query = nextURL.Query()
	}
}

func cloneValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, items := range values {
		cloned[key] = append([]string(nil), items...)
	}

	return cloned
}
