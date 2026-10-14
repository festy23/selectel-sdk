package v2

import (
	"net/url"
	"strconv"
)

// SelectionOptions contains query parameters shared by all List operations.
type SelectionOptions struct {
	Filters map[string][]string
	Fields  []string
	SortKey string
	SortDir string
}

// Values encodes selection parameters using repeated keys for repeated values.
func (options SelectionOptions) Values() url.Values {
	values := make(url.Values)
	for key, items := range options.Filters {
		for _, item := range items {
			values.Add(key, item)
		}
	}
	for _, field := range options.Fields {
		values.Add("fields", field)
	}
	if options.SortKey != "" {
		values.Set("sort_key", options.SortKey)
	}
	if options.SortDir != "" {
		values.Set("sort_dir", options.SortDir)
	}

	return values
}

// ListOptions adds pagination parameters to SelectionOptions.
type ListOptions struct {
	SelectionOptions
	Limit  int
	Marker string
}

// Values encodes selection and pagination parameters.
func (options ListOptions) Values() url.Values {
	values := options.SelectionOptions.Values()
	if options.Limit > 0 {
		values.Set("limit", strconv.Itoa(options.Limit))
	}
	if options.Marker != "" {
		values.Set("marker", options.Marker)
	}

	return values
}
