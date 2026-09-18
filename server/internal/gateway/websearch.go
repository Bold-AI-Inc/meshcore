package gateway

import (
	"mesh-server/internal/providerfamily"
)

const maxCitations = 100

type searchAccumulator struct {
	index     map[string]int
	citations []providerfamily.Citation

	queries    []string
	querySeen  map[string]bool
	reported   int
	suggestion string

	truncated bool
}

func newSearchAccumulator() *searchAccumulator {
	return &searchAccumulator{
		index:     make(map[string]int),
		querySeen: make(map[string]bool),
	}
}

func (a *searchAccumulator) add(ev providerfamily.SearchEvent) {
	for _, c := range ev.Citations {
		if c.URL == "" {
			continue
		}
		if pos, ok := a.index[c.URL]; ok {
			a.merge(&a.citations[pos], c)
			continue
		}
		if len(a.citations) >= maxCitations {
			a.truncated = true
			continue
		}
		a.index[c.URL] = len(a.citations)
		a.citations = append(a.citations, c)
	}

	for _, q := range ev.Queries {
		if q == "" || a.querySeen[q] {
			continue
		}
		a.querySeen[q] = true
		a.queries = append(a.queries, q)
	}

	if ev.Searches > a.reported {
		a.reported = ev.Searches
	}

	if ev.RenderedSuggestions != "" {
		a.suggestion = ev.RenderedSuggestions
	}
}

func (a *searchAccumulator) merge(dst *providerfamily.Citation, src providerfamily.Citation) {
	if dst.Title == "" {
		dst.Title = src.Title
	}
	if dst.CitedText == "" {
		dst.CitedText = src.CitedText
	}
	if dst.PageAge == "" {
		dst.PageAge = src.PageAge
	}
}

func (a *searchAccumulator) used() bool {
	return len(a.citations) > 0 || len(a.queries) > 0 || a.reported > 0
}

func (a *searchAccumulator) billableSearches() int {
	if a.reported > 0 {
		return a.reported
	}
	if len(a.queries) > 0 {
		return len(a.queries)
	}
	if len(a.citations) > 0 {
		return 1
	}
	return 0
}

func (a *searchAccumulator) result() map[string]any {
	if !a.used() {
		return nil
	}
	out := map[string]any{
		"sources":  a.citations,
		"searches": a.billableSearches(),
	}
	if a.citations == nil {
		out["sources"] = []providerfamily.Citation{}
	}
	if len(a.queries) > 0 {
		out["queries"] = a.queries
	}
	if a.suggestion != "" {
		out["search_suggestions_html"] = a.suggestion
	}
	if a.truncated {
		out["sources_truncated"] = true
	}
	return out
}
