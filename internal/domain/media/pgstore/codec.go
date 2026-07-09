package pgstore

import (
	"encoding/json"
	"fmt"
)

type rowScanner interface {
	Scan(dest ...any) error
}

func decodeMetadata(raw string) (map[string]string, error) {
	if raw == "" {
		return nil, nil
	}

	var metadata map[string]string
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, fmt.Errorf("decode media metadata: %w", err)
	}

	return metadata, nil
}

func encodeMetadata(metadata map[string]string) (string, error) {
	if len(metadata) == 0 {
		return "{}", nil
	}

	raw, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("encode media metadata: %w", err)
	}

	return string(raw), nil
}
