package psaportal

import (
	"encoding/json"
	"fmt"
)

type envelope struct {
	Type  string `json:"type"`
	Nodes []struct {
		Type string            `json:"type"`
		Data []json.RawMessage `json:"data"`
	} `json:"nodes"`
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
		return nil, fmt.Errorf("psaportal: no data node in __data.json")
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
	if idx == -1 {
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
