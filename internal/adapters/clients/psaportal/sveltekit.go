package psaportal

import (
	"encoding/json"
	"fmt"
	"net/url"
)

type envelope struct {
	Type     string          `json:"type"`
	Location json.RawMessage `json:"location"`
	Nodes    []struct {
		Type string            `json:"type"`
		Data []json.RawMessage `json:"data"`
	} `json:"nodes"`
}

// missingDataError reports envelope metadata, never node contents or page bodies.
func (env envelope) missingDataError() error {
	types := make([]string, 0, min(len(env.Nodes), 8))
	for _, node := range env.Nodes[:min(len(env.Nodes), 8)] {
		types = append(types, truncateRunes(node.Type, 64))
	}
	if len(env.Nodes) > 8 {
		types = append(types, "…")
	}
	return fmt.Errorf("psaportal: no data node in envelope (type=%q, nodes=%q, redirect=%q)",
		truncateRunes(env.Type, 64), types, safeRedirectTarget(env.Location))
}

// safeRedirectTarget decodes optional diagnostic metadata only on failure, so
// an unrelated location field cannot break successful data decoding. Only known
// public route paths are shown: arbitrary paths can contain session tokens too.
func safeRedirectTarget(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var location string
	if err := json.Unmarshal(raw, &location); err != nil {
		return "[redacted]"
	}
	if location == "" {
		return ""
	}
	u, err := url.Parse(location)
	if err != nil || u.Opaque != "" || (u.Scheme != "" && u.Scheme != "https" && u.Scheme != "http") {
		return "[redacted]"
	}
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	switch u.Path {
	case "", "/", "/signin", "/campaigns", "/analytics":
	default:
		u.Path = "/REDACTED"
	}
	u.RawPath = ""
	if u.Host == "" && u.Path == "" {
		return "[redacted]"
	}
	return truncateRunes(u.String(), 256)
}

// DecodeSvelteKitValue resolves a SvelteKit flattened __data.json payload and
// returns the JSON value stored under topKey of the root object.
func DecodeSvelteKitValue(raw []byte, topKey string) (json.RawMessage, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("psaportal: decode envelope: %w", err)
	}
	var data []json.RawMessage
	for i := len(env.Nodes) - 1; i >= 0; i-- {
		if env.Nodes[i].Type == "data" && env.Nodes[i].Data != nil {
			data = env.Nodes[i].Data
			break
		}
	}
	if data == nil {
		return nil, env.missingDataError()
	}
	resolved, err := unflatten(data, 0, map[int]json.RawMessage{})
	if err != nil {
		return nil, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(resolved, &root); err != nil {
		return nil, fmt.Errorf("psaportal: root is not an object: %w", err)
	}
	v, ok := root[topKey]
	if !ok {
		return nil, fmt.Errorf("psaportal: key %q not found in __data.json root", topKey)
	}
	return v, nil
}

func unflatten(data []json.RawMessage, idx int, memo map[int]json.RawMessage) (json.RawMessage, error) {
	// All devalue sentinels collapse to null on this path. DecodeSvelteKitValue
	// hands raw JSON bytes to its caller and never re-encodes them, so there is
	// no round trip to corrupt — but erroring here would abort an otherwise
	// good decode. DecodeRefPacked/resolveValue is the fidelity-preserving path.
	if _, ok := sentinelFor(idx); ok {
		return json.RawMessage("null"), nil
	}
	if idx < 0 || idx >= len(data) {
		return nil, fmt.Errorf("psaportal: pointer %d out of range", idx)
	}
	if v, ok := memo[idx]; ok {
		return v, nil
	}
	memo[idx] = json.RawMessage("null") // cycle guard
	raw := data[idx]
	if len(raw) > 0 && raw[0] == '{' {
		var obj map[string]int
		if err := json.Unmarshal(raw, &obj); err == nil {
			out := map[string]json.RawMessage{}
			for k, ptr := range obj {
				rv, err := unflatten(data, ptr, memo)
				if err != nil {
					return nil, err
				}
				out[k] = rv
			}
			b, err := json.Marshal(out)
			if err != nil {
				return nil, fmt.Errorf("psaportal: marshal object at %d: %w", idx, err)
			}
			memo[idx] = b
			return b, nil
		}
	}
	if len(raw) > 0 && raw[0] == '[' {
		var arr []int
		if err := json.Unmarshal(raw, &arr); err == nil {
			out := make([]json.RawMessage, 0, len(arr))
			for _, ptr := range arr {
				rv, err := unflatten(data, ptr, memo)
				if err != nil {
					return nil, err
				}
				out = append(out, rv)
			}
			b, err := json.Marshal(out)
			if err != nil {
				return nil, fmt.Errorf("psaportal: marshal array at %d: %w", idx, err)
			}
			memo[idx] = b
			return b, nil
		}
	}
	memo[idx] = raw
	return raw, nil
}
