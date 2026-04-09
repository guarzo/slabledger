package azureai

import "encoding/json"

// ensureProperties ensures all "object" schemas in a tool's parameters have
// a "properties" key. The Responses API rejects schemas with missing properties.
// Works recursively on nested schemas. Accepts the Parameters field (any type)
// and returns a corrected copy.
func ensureProperties(params any) any {
	if params == nil {
		return nil
	}
	m, ok := params.(map[string]any)
	if ok {
		return ensurePropertiesMap(m)
	}
	// Try JSON-roundtrip for struct types (like jsonSchema from advisortool).
	b, err := json.Marshal(params)
	if err != nil {
		return params
	}
	var generic map[string]any
	if err := json.Unmarshal(b, &generic); err != nil {
		return params
	}
	return ensurePropertiesMap(generic)
}

func ensurePropertiesMap(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	// If type=object, ensure properties exists.
	if typ, ok := result["type"].(string); ok && typ == "object" {
		if _, ok := result["properties"]; !ok {
			result["properties"] = map[string]any{}
		} else if result["properties"] == nil {
			result["properties"] = map[string]any{}
		}
	}
	// Recurse into properties values.
	if props, ok := result["properties"].(map[string]any); ok {
		fixed := make(map[string]any, len(props))
		for k, v := range props {
			if sub, ok := v.(map[string]any); ok {
				fixed[k] = ensurePropertiesMap(sub)
			} else {
				fixed[k] = v
			}
		}
		result["properties"] = fixed
	}
	// Recurse into "items" (array element schema).
	if items, ok := result["items"]; ok {
		switch v := items.(type) {
		case map[string]any:
			result["items"] = ensurePropertiesMap(v)
		case []any:
			fixed := make([]any, len(v))
			for i, item := range v {
				if sub, ok := item.(map[string]any); ok {
					fixed[i] = ensurePropertiesMap(sub)
				} else {
					fixed[i] = item
				}
			}
			result["items"] = fixed
		}
	}
	// Recurse into "additionalProperties" if it's a schema.
	if ap, ok := result["additionalProperties"]; ok {
		if sub, ok := ap.(map[string]any); ok {
			result["additionalProperties"] = ensurePropertiesMap(sub)
		}
	}
	// Recurse into combinators: anyOf, oneOf, allOf.
	for _, key := range []string{"anyOf", "oneOf", "allOf"} {
		if arr, ok := result[key].([]any); ok {
			fixed := make([]any, len(arr))
			for i, item := range arr {
				if sub, ok := item.(map[string]any); ok {
					fixed[i] = ensurePropertiesMap(sub)
				} else {
					fixed[i] = item
				}
			}
			result[key] = fixed
		}
	}
	return result
}
