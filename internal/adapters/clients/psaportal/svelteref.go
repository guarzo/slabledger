package psaportal

import (
	"encoding/json"
	"fmt"
)

// DecodeRefPacked resolves a SvelteKit reference-packed array (root at index 0)
// into a plain Go value (map[string]any / []any / scalar / Sentinel).
func DecodeRefPacked(data []json.RawMessage) (any, error) {
	return resolveValue(data, 0, map[int]any{})
}

// resolveValue resolves slot idx of a ref-packed array into a plain Go value.
//
// It is a value-based sibling of unflatten rather than a wrapper around it:
// unflatten returns JSON bytes, and devalue's sentinels (undefined, NaN, ±Inf)
// have no JSON representation, so they cannot survive that bottleneck. The two
// resolvers are kept separate deliberately — unflatten serves
// DecodeSvelteKitValue, which returns raw bytes to its caller and never
// re-encodes, so it has no fidelity requirement.
func resolveValue(data []json.RawMessage, idx int, memo map[int]any) (any, error) {
	if s, ok := sentinelFor(idx); ok {
		return s, nil
	}
	if idx < 0 || idx >= len(data) {
		return nil, fmt.Errorf("psaportal: pointer %d out of range", idx)
	}
	if v, ok := memo[idx]; ok {
		return v, nil
	}
	memo[idx] = nil // cycle guard, mirroring unflatten
	raw := data[idx]

	if len(raw) > 0 && raw[0] == '{' {
		var obj map[string]int
		if err := json.Unmarshal(raw, &obj); err == nil {
			out := make(map[string]any, len(obj))
			for k, ptr := range obj {
				rv, err := resolveValue(data, ptr, memo)
				if err != nil {
					return nil, err
				}
				out[k] = rv
			}
			memo[idx] = out
			return out, nil
		}
	}
	if len(raw) > 0 && raw[0] == '[' {
		var arr []int
		if err := json.Unmarshal(raw, &arr); err == nil {
			out := make([]any, 0, len(arr))
			for _, ptr := range arr {
				rv, err := resolveValue(data, ptr, memo)
				if err != nil {
					return nil, err
				}
				out = append(out, rv)
			}
			memo[idx] = out
			return out, nil
		}
	}

	var scalar any
	if err := json.Unmarshal(raw, &scalar); err != nil {
		return nil, fmt.Errorf("psaportal: decode slot %d: %w", idx, err)
	}
	memo[idx] = scalar
	return scalar, nil
}

// EncodeRefPacked packs v into a SvelteKit reference array with root at index 0.
// Scalars are de-duplicated by their JSON encoding; objects/arrays get fresh
// slots. The server derefs by index, so byte-identity with PSA's own encoder is
// not required — only self-consistency.
func EncodeRefPacked(v any) ([]json.RawMessage, error) {
	e := &refEncoder{index: map[string]int{}}
	if _, err := e.add(v); err != nil {
		return nil, err
	}
	return e.out, nil
}

type refEncoder struct {
	out   []json.RawMessage
	index map[string]int // scalar JSON -> slot, for de-dup
}

func (e *refEncoder) add(v any) (int, error) {
	switch t := v.(type) {
	case map[string]any:
		slot := e.reserve()
		obj := make(map[string]int, len(t))
		for k, val := range t {
			ref, err := e.add(val)
			if err != nil {
				return 0, err
			}
			obj[k] = ref
		}
		b, err := json.Marshal(obj)
		if err != nil {
			return 0, fmt.Errorf("psaportal: encode object: %w", err)
		}
		e.out[slot] = b
		return slot, nil
	case []any:
		slot := e.reserve()
		arr := make([]int, len(t))
		for i, val := range t {
			ref, err := e.add(val)
			if err != nil {
				return 0, err
			}
			arr[i] = ref
		}
		b, err := json.Marshal(arr)
		if err != nil {
			return 0, fmt.Errorf("psaportal: encode array: %w", err)
		}
		e.out[slot] = b
		return slot, nil
	default: // scalar (string/float64/bool/nil)
		b, err := json.Marshal(t)
		if err != nil {
			return 0, fmt.Errorf("psaportal: encode scalar: %w", err)
		}
		key := string(b)
		if s, ok := e.index[key]; ok {
			return s, nil
		}
		slot := e.reserve()
		e.out[slot] = b
		e.index[key] = slot
		return slot, nil
	}
}

func (e *refEncoder) reserve() int {
	e.out = append(e.out, nil)
	return len(e.out) - 1
}

// packedFromEnvelope extracts the last data node's flat array from a
// SvelteKit __data.json envelope (skipping "skip" nodes).
func packedFromEnvelope(raw []byte) ([]json.RawMessage, error) {
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("psaportal: decode envelope: %w", err)
	}
	for i := len(env.Nodes) - 1; i >= 0; i-- {
		if env.Nodes[i].Type == "data" && env.Nodes[i].Data != nil {
			return env.Nodes[i].Data, nil
		}
	}
	return nil, fmt.Errorf("psaportal: no data node in envelope")
}
