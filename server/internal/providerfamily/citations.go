package providerfamily

type Citation struct {
	URL       string `json:"url"`
	Title     string `json:"title,omitempty"`
	CitedText string `json:"cited_text,omitempty"`
	PageAge   string `json:"page_age,omitempty"`
}

type SearchEvent struct {
	Citations []Citation

	Queries []string

	Searches int

	RenderedSuggestions string
}

func (e SearchEvent) Empty() bool {
	return len(e.Citations) == 0 && len(e.Queries) == 0 && e.Searches == 0 && e.RenderedSuggestions == ""
}

func ExtractSearchEvent(family string, event any) SearchEvent {
	obj, ok := event.(map[string]any)
	if !ok {
		return SearchEvent{}
	}
	switch family {
	case Anthropic:
		return extractAnthropic(obj)
	case OpenAI, Ollama:
		if ev := extractOpenAIChat(obj); !ev.Empty() {
			return ev
		}
		return extractOpenAIResponses(obj)
	case OpenAIResponses:
		return extractOpenAIResponses(obj)
	case Gemini:
		return extractGemini(obj)
	default:
		return SearchEvent{}
	}
}

func extractAnthropic(obj map[string]any) SearchEvent {
	var out SearchEvent

	switch obj["type"] {
	case "content_block_start":
		block, ok := obj["content_block"].(map[string]any)
		if !ok || block["type"] != "web_search_tool_result" {
			return out
		}
		results, ok := block["content"].([]any)
		if !ok {
			return out
		}
		for _, r := range results {
			res, ok := r.(map[string]any)
			if !ok || res["type"] != "web_search_result" {
				continue
			}
			if url, _ := res["url"].(string); url != "" {
				title, _ := res["title"].(string)
				pageAge, _ := res["page_age"].(string)
				out.Citations = append(out.Citations, Citation{URL: url, Title: title, PageAge: pageAge})
			}
		}

	case "content_block_delta":
		delta, ok := obj["delta"].(map[string]any)
		if !ok || delta["type"] != "citations_delta" {
			return out
		}
		cit, ok := delta["citation"].(map[string]any)
		if !ok {
			return out
		}
		if url, _ := cit["url"].(string); url != "" {
			title, _ := cit["title"].(string)
			citedText, _ := cit["cited_text"].(string)
			out.Citations = append(out.Citations, Citation{URL: url, Title: title, CitedText: citedText})
		}
	}

	if n, ok := anthropicSearchRequests(obj["usage"]); ok {
		out.Searches = n
	} else if msg, ok := obj["message"].(map[string]any); ok {
		if n, ok := anthropicSearchRequests(msg["usage"]); ok {
			out.Searches = n
		}
	}

	return out
}

func anthropicSearchRequests(usage any) (int, bool) {
	u, ok := usage.(map[string]any)
	if !ok {
		return 0, false
	}
	stu, ok := u["server_tool_use"].(map[string]any)
	if !ok {
		return 0, false
	}
	n, ok := stu["web_search_requests"].(float64)
	if !ok {
		return 0, false
	}
	return int(n), true
}

func extractOpenAIChat(obj map[string]any) SearchEvent {
	var out SearchEvent

	choices, ok := obj["choices"].([]any)
	if !ok {
		return out
	}
	for _, c := range choices {
		choice, ok := c.(map[string]any)
		if !ok {
			continue
		}
		annotations := annotationsOf(choice["delta"])
		if annotations == nil {
			annotations = annotationsOf(choice["message"])
		}
		for _, a := range annotations {
			ann, ok := a.(map[string]any)
			if !ok || ann["type"] != "url_citation" {
				continue
			}
			fields := ann
			if nested, ok := ann["url_citation"].(map[string]any); ok {
				fields = nested
			}
			if url, _ := fields["url"].(string); url != "" {
				title, _ := fields["title"].(string)
				out.Citations = append(out.Citations, Citation{URL: url, Title: title})
			}
		}
	}
	return out
}

func extractOpenAIResponses(obj map[string]any) SearchEvent {
	var out SearchEvent

	switch obj["type"] {
	case "response.output_text.annotation.added":
		appendURLCitation(&out, obj["annotation"])
		return out

	case "response.output_item.added", "response.output_item.done":
		collectResponsesItem(&out, obj["item"])
		return out
	}

	output, ok := obj["output"].([]any)
	if !ok {
		if resp, isResp := obj["response"].(map[string]any); isResp {
			output, ok = resp["output"].([]any)
		}
	}
	if ok {
		for _, item := range output {
			collectResponsesItem(&out, item)
		}
	}
	return out
}

func collectResponsesItem(out *SearchEvent, item any) {
	it, ok := item.(map[string]any)
	if !ok {
		return
	}
	switch it["type"] {
	case "web_search_call":
		if action, ok := it["action"].(map[string]any); ok {
			if q, _ := action["query"].(string); q != "" {
				out.Queries = append(out.Queries, q)
			}
		}
	case "message":
		content, ok := it["content"].([]any)
		if !ok {
			return
		}
		for _, part := range content {
			p, ok := part.(map[string]any)
			if !ok {
				continue
			}
			annotations, _ := p["annotations"].([]any)
			for _, a := range annotations {
				appendURLCitation(out, a)
			}
		}
	}
}

func appendURLCitation(out *SearchEvent, a any) {
	ann, ok := a.(map[string]any)
	if !ok || ann["type"] != "url_citation" {
		return
	}
	fields := ann
	if nested, ok := ann["url_citation"].(map[string]any); ok {
		fields = nested
	}
	if url, _ := fields["url"].(string); url != "" {
		title, _ := fields["title"].(string)
		out.Citations = append(out.Citations, Citation{URL: url, Title: title})
	}
}

func annotationsOf(v any) []any {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	annotations, _ := m["annotations"].([]any)
	return annotations
}

func extractGemini(obj map[string]any) SearchEvent {
	var out SearchEvent

	candidates, ok := obj["candidates"].([]any)
	if !ok {
		return out
	}
	for _, c := range candidates {
		cand, ok := c.(map[string]any)
		if !ok {
			continue
		}
		meta, ok := cand["groundingMetadata"].(map[string]any)
		if !ok {
			continue
		}

		if chunks, ok := meta["groundingChunks"].([]any); ok {
			for _, ch := range chunks {
				chunk, ok := ch.(map[string]any)
				if !ok {
					continue
				}
				web, ok := chunk["web"].(map[string]any)
				if !ok {
					continue
				}
				if uri, _ := web["uri"].(string); uri != "" {
					title, _ := web["title"].(string)
					out.Citations = append(out.Citations, Citation{URL: uri, Title: title})
				}
			}
		}

		if queries, ok := meta["webSearchQueries"].([]any); ok {
			for _, q := range queries {
				if s, _ := q.(string); s != "" {
					out.Queries = append(out.Queries, s)
				}
			}
		}

		if entry, ok := meta["searchEntryPoint"].(map[string]any); ok {
			if rendered, _ := entry["renderedContent"].(string); rendered != "" {
				out.RenderedSuggestions = rendered
			}
		}
	}
	return out
}

func SupportsWebSearch(family string) bool {
	switch family {
	case Anthropic, OpenAI, OpenAIResponses, Gemini, Ollama:
		return true
	default:
		return false
	}
}
