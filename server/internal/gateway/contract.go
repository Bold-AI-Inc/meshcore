package gateway

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"

	"mesh-server/internal/providerfamily"
)

const (
	maxImageBytes    = 20 << 20
	maxDocumentBytes = 50 << 20
	maxAudioBytes    = 75 << 20
	maxVideoBytes    = 200 << 20
)

type contentBlockRequest struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`
	MediaType string `json:"media_type,omitempty"`
	Data      string `json:"data,omitempty"`
	URL       string `json:"url,omitempty"`
}

const defaultMaxTokens = 4096

type proxyRequest struct {
	ID        string                `json:"id,omitempty"`
	Query     string                `json:"query,omitempty"`
	Content   []contentBlockRequest `json:"content,omitempty"`
	MaxTokens *int                  `json:"max_tokens,omitempty"`

	WebSearch bool `json:"web_search,omitempty"`
}

func decodeProxyRequest(r *http.Request, dst *proxyRequest) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

var (
	errBothQueryAndContent = errors.New("request must set either query or content, not both")
	errEmptyRequest        = errors.New("request must set either query or content")
	errInvalidMaxTokens    = errors.New("max_tokens must be a positive integer")
	errUnknownBlockType    = errors.New("unknown content block type")
	errBlockTooLarge       = errors.New("content block exceeds the size limit for its type")
	errMissingBlockData    = errors.New("content block must set either data or url")
	errMissingMediaType    = errors.New("non-text content block must set media_type")
)

func resolveContent(req proxyRequest) ([]providerfamily.Block, error) {
	if req.Query != "" && len(req.Content) > 0 {
		return nil, errBothQueryAndContent
	}
	if req.Query == "" && len(req.Content) == 0 {
		return nil, errEmptyRequest
	}

	if req.Query != "" {
		return []providerfamily.Block{{Type: providerfamily.BlockText, Text: req.Query}}, nil
	}

	blocks := make([]providerfamily.Block, 0, len(req.Content))
	for _, cb := range req.Content {
		block := providerfamily.Block{
			Type:      providerfamily.BlockType(cb.Type),
			Text:      cb.Text,
			MediaType: cb.MediaType,
			Data:      cb.Data,
			URL:       cb.URL,
		}

		var maxBytes int
		switch block.Type {
		case providerfamily.BlockText:
			blocks = append(blocks, block)
			continue
		case providerfamily.BlockImage:
			maxBytes = maxImageBytes
		case providerfamily.BlockDocument:
			maxBytes = maxDocumentBytes
		case providerfamily.BlockAudio:
			maxBytes = maxAudioBytes
		case providerfamily.BlockVideo:
			maxBytes = maxVideoBytes
		default:
			return nil, errUnknownBlockType
		}

		if block.Data == "" && block.URL == "" {
			return nil, errMissingBlockData
		}
		if block.Data != "" && block.MediaType == "" {
			return nil, errMissingMediaType
		}
		if block.Data != "" && decodedBase64Len(block.Data) > maxBytes {
			return nil, errBlockTooLarge
		}

		blocks = append(blocks, block)
	}
	return blocks, nil
}

func decodedBase64Len(s string) int {
	return base64.StdEncoding.DecodedLen(len(s))
}

type chunkWriter struct {
	prefix []byte
	buf    []byte
}

func newChunkWriter(requestID string) *chunkWriter {
	id, err := json.Marshal(requestID)
	if err != nil {
		id = []byte(`""`)
	}
	prefix := make([]byte, 0, len(id)+16)
	prefix = append(prefix, `{"id":`...)
	prefix = append(prefix, id...)
	prefix = append(prefix, `,"delta":`...)
	return &chunkWriter{prefix: prefix, buf: make([]byte, 0, 1024)}
}

func (c *chunkWriter) writeDelta(w http.ResponseWriter, delta string) {
	c.buf = c.buf[:0]
	c.buf = append(c.buf, c.prefix...)
	c.buf = appendJSONString(c.buf, delta)
	c.buf = append(c.buf, '}', '\n')
	_, _ = w.Write(c.buf)
}

func appendJSONString(dst []byte, s string) []byte {
	dst = append(dst, '"')
	start := 0
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= 0x20 && b != '"' && b != '\\' {
			continue
		}
		dst = append(dst, s[start:i]...)
		switch b {
		case '"':
			dst = append(dst, '\\', '"')
		case '\\':
			dst = append(dst, '\\', '\\')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		default:
			dst = append(dst, `\u00`...)
			dst = append(dst, hexDigits[b>>4], hexDigits[b&0xF])
		}
		start = i + 1
	}
	dst = append(dst, s[start:]...)
	return append(dst, '"')
}

const hexDigits = "0123456789abcdef"

func writeProxyDoneChunk(w http.ResponseWriter, requestID string, tokensIn, tokensOut, maxTokens int, search map[string]any, incomplete bool) {
	usage := map[string]any{
		"input_tokens":  tokensIn,
		"output_tokens": tokensOut,
	}
	chunk := map[string]any{
		"id":    requestID,
		"done":  true,
		"usage": usage,
	}
	if maxTokens > 0 && tokensOut >= maxTokens {
		chunk["truncated"] = true
	}
	if incomplete {
		chunk["incomplete"] = true
	}
	if search != nil {
		if n, ok := search["searches"]; ok {
			usage["web_searches"] = n
		}
		for k, v := range search {
			if k == "searches" {
				continue
			}
			chunk[k] = v
		}
	}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	_, _ = w.Write(encoded)
	_, _ = w.Write([]byte("\n"))
}

func writeSourcesChunk(w http.ResponseWriter, requestID string, sources []providerfamily.Citation) {
	chunk := map[string]any{
		"id":      requestID,
		"sources": sources,
	}
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	_, _ = w.Write(encoded)
	_, _ = w.Write([]byte("\n"))
}
