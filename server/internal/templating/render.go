package templating

import (
	"bytes"
	"encoding/json"
	"regexp"
)

var placeholderRe = regexp.MustCompile(`\{\{([A-Za-z0-9_]+)\}\}`)

func RenderString(tmpl string, vars map[string]string) string {
	return placeholderRe.ReplaceAllStringFunc(tmpl, func(m string) string {
		if val, ok := vars[m[2:len(m)-2]]; ok {
			return val
		}
		return m
	})
}

func Render(tmpl []byte, vars map[string]string) ([]byte, error) {
	var firstErr error
	out := placeholderRe.ReplaceAllFunc(tmpl, func(m []byte) []byte {
		val, ok := vars[string(m[2:len(m)-2])]
		if !ok {
			return m
		}
		encoded, err := json.Marshal(val)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return m
		}
		return encoded[1 : len(encoded)-1]
	})
	if firstErr != nil {
		return nil, firstErr
	}
	return out, nil
}

func References(tmpl []byte, key string) bool {
	return bytes.Contains(tmpl, []byte("{{"+key+"}}"))
}

func RenderRaw(tmpl []byte, rawVars map[string]json.RawMessage) []byte {
	out := tmpl
	for key, val := range rawVars {
		placeholder := []byte(`"{{` + key + `}}"`)
		out = bytes.ReplaceAll(out, placeholder, val)
	}
	return out
}

func RenderHeaders(tmpl []byte, vars map[string]string) (map[string]string, error) {
	rendered, err := Render(tmpl, vars)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{}
	if err := json.Unmarshal(rendered, &headers); err != nil {
		return nil, err
	}
	return headers, nil
}
