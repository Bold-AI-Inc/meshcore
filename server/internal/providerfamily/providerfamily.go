package providerfamily

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	Generic   = "generic"
	Anthropic = "anthropic"
	OpenAI    = "openai"
	Gemini    = "gemini"
	Ollama    = "ollama"

	OpenAIResponses = "openai_responses"
)

type BlockType string

const (
	BlockText     BlockType = "text"
	BlockImage    BlockType = "image"
	BlockDocument BlockType = "document"
	BlockAudio    BlockType = "audio"
	BlockVideo    BlockType = "video"
)

type Block struct {
	Type      BlockType
	Text      string
	MediaType string
	Data      string
	URL       string
}

type UnsupportedBlockError struct {
	Family string
	Type   BlockType
}

func (e *UnsupportedBlockError) Error() string {
	return fmt.Sprintf("provider family %q does not support content type %q", e.Family, e.Type)
}

func Known(family string) bool {
	switch family {
	case Generic, Anthropic, OpenAI, OpenAIResponses, Gemini, Ollama:
		return true
	default:
		return false
	}
}

func PlainText(blocks []Block) string {
	out := ""
	for _, b := range blocks {
		if b.Type != BlockText {
			continue
		}
		if out != "" {
			out += "\n\n"
		}
		out += b.Text
	}
	return out
}

func BuildContentJSON(family string, blocks []Block) (json.RawMessage, error) {
	switch family {
	case Anthropic:
		return buildAnthropic(blocks)
	case OpenAI:
		return buildOpenAI(blocks)
	case OpenAIResponses:
		return buildOpenAIResponses(blocks)
	case Ollama:
		return buildOllama(blocks)
	case Gemini:
		return buildGemini(blocks)
	case Generic:
		return json.Marshal([]any{})
	default:
		return nil, fmt.Errorf("unknown provider family %q", family)
	}
}

func buildAnthropic(blocks []Block) (json.RawMessage, error) {
	out := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case BlockText:
			out = append(out, map[string]any{"type": "text", "text": b.Text})
		case BlockImage, BlockDocument:
			out = append(out, map[string]any{
				"type":   string(b.Type),
				"source": anthropicSource(b),
			})
		default:
			return nil, &UnsupportedBlockError{Family: Anthropic, Type: b.Type}
		}
	}
	return json.Marshal(out)
}

func anthropicSource(b Block) map[string]any {
	if b.URL != "" {
		return map[string]any{"type": "url", "url": b.URL}
	}
	return map[string]any{"type": "base64", "media_type": b.MediaType, "data": b.Data}
}

func buildOpenAI(blocks []Block) (json.RawMessage, error) {
	out := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case BlockText:
			out = append(out, map[string]any{"type": "text", "text": b.Text})
		case BlockImage:
			out = append(out, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": openAIImageURL(b)},
			})
		case BlockDocument:
			block, err := openAIFileBlock(b)
			if err != nil {
				return nil, err
			}
			out = append(out, block)
		case BlockAudio:
			block, err := openAIAudioBlock(b)
			if err != nil {
				return nil, err
			}
			out = append(out, block)
		default:
			return nil, &UnsupportedBlockError{Family: OpenAI, Type: b.Type}
		}
	}
	return json.Marshal(out)
}

func buildOpenAIResponses(blocks []Block) (json.RawMessage, error) {
	out := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case BlockText:
			out = append(out, map[string]any{"type": "input_text", "text": b.Text})
		case BlockImage:
			out = append(out, map[string]any{
				"type":      "input_image",
				"image_url": openAIImageURL(b),
			})
		case BlockDocument:
			if b.URL != "" {
				return nil, &UnsupportedBlockError{Family: OpenAIResponses, Type: b.Type}
			}
			out = append(out, map[string]any{
				"type":      "input_file",
				"filename":  "document" + extensionForMediaType(b.MediaType),
				"file_data": fmt.Sprintf("data:%s;base64,%s", b.MediaType, b.Data),
			})
		default:
			return nil, &UnsupportedBlockError{Family: OpenAIResponses, Type: b.Type}
		}
	}
	return json.Marshal(out)
}

func openAIImageURL(b Block) string {
	if b.URL != "" {
		return b.URL
	}
	return fmt.Sprintf("data:%s;base64,%s", b.MediaType, b.Data)
}

func openAIFileBlock(b Block) (map[string]any, error) {
	if b.URL != "" {
		return nil, fmt.Errorf("openai document blocks must be inline (data), not url: %w",
			&UnsupportedBlockError{Family: OpenAI, Type: BlockDocument})
	}
	return map[string]any{
		"type": "file",
		"file": map[string]any{
			"filename":  "document" + extensionForMediaType(b.MediaType),
			"file_data": fmt.Sprintf("data:%s;base64,%s", b.MediaType, b.Data),
		},
	}, nil
}

func openAIAudioBlock(b Block) (map[string]any, error) {
	if b.URL != "" {
		return nil, fmt.Errorf("openai audio blocks must be inline (data), not url: %w",
			&UnsupportedBlockError{Family: OpenAI, Type: BlockAudio})
	}
	format, ok := openAIAudioFormat(b.MediaType)
	if !ok {
		return nil, fmt.Errorf("openai audio input only supports wav/mp3, got media_type %q: %w",
			b.MediaType, &UnsupportedBlockError{Family: OpenAI, Type: BlockAudio})
	}
	return map[string]any{
		"type": "input_audio",
		"input_audio": map[string]any{
			"data":   b.Data,
			"format": format,
		},
	}, nil
}

func openAIAudioFormat(mediaType string) (string, bool) {
	switch mediaType {
	case "audio/mpeg", "audio/mp3":
		return "mp3", true
	case "audio/wav", "audio/x-wav", "audio/wave":
		return "wav", true
	default:
		return "", false
	}
}

func extensionForMediaType(mediaType string) string {
	if i := strings.LastIndex(mediaType, "/"); i >= 0 {
		return "." + mediaType[i+1:]
	}
	return ""
}

func buildOllama(blocks []Block) (json.RawMessage, error) {
	out := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case BlockText:
			out = append(out, map[string]any{"type": "text", "text": b.Text})
		case BlockImage:
			out = append(out, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": openAIImageURL(b)},
			})
		default:
			return nil, &UnsupportedBlockError{Family: Ollama, Type: b.Type}
		}
	}
	return json.Marshal(out)
}

func buildGemini(blocks []Block) (json.RawMessage, error) {
	out := make([]map[string]any, 0, len(blocks))
	for _, b := range blocks {
		switch b.Type {
		case BlockText:
			out = append(out, map[string]any{"text": b.Text})
		case BlockImage, BlockDocument, BlockAudio, BlockVideo:
			if b.URL != "" {
				out = append(out, map[string]any{
					"file_data": map[string]any{"mime_type": b.MediaType, "file_uri": b.URL},
				})
			} else {
				out = append(out, map[string]any{
					"inline_data": map[string]any{"mime_type": b.MediaType, "data": b.Data},
				})
			}
		default:
			return nil, &UnsupportedBlockError{Family: Gemini, Type: b.Type}
		}
	}
	return json.Marshal(out)
}
