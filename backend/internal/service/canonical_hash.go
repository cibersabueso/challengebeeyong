package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

// CanonicalHash returns a deterministic SHA-256 hex digest of a JSON payload.
// Two payloads that are semantically equal (same keys and values, regardless of
// whitespace or key order) produce the same hash. This is used to detect
// idempotency-key reuse with a different payload (AC-007 / IDEMPOTENCY_CONFLICT).
func CanonicalHash(payload []byte) (string, error) {
	if len(payload) == 0 {
		return "", fmt.Errorf("empty payload")
	}

	var v any
	if err := json.Unmarshal(payload, &v); err != nil {
		return "", fmt.Errorf("payload is not valid json: %w", err)
	}

	canonical, err := canonicalize(v)
	if err != nil {
		return "", fmt.Errorf("canonicalize: %w", err)
	}

	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func canonicalize(v any) ([]byte, error) {
	switch typed := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for k := range typed {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		buf := []byte{'{'}
		for i, k := range keys {
			if i > 0 {
				buf = append(buf, ',')
			}
			keyJSON, err := json.Marshal(k)
			if err != nil {
				return nil, err
			}
			buf = append(buf, keyJSON...)
			buf = append(buf, ':')
			valJSON, err := canonicalize(typed[k])
			if err != nil {
				return nil, err
			}
			buf = append(buf, valJSON...)
		}
		buf = append(buf, '}')
		return buf, nil

	case []any:
		buf := []byte{'['}
		for i, e := range typed {
			if i > 0 {
				buf = append(buf, ',')
			}
			eJSON, err := canonicalize(e)
			if err != nil {
				return nil, err
			}
			buf = append(buf, eJSON...)
		}
		buf = append(buf, ']')
		return buf, nil

	default:
		return json.Marshal(typed)
	}
}
