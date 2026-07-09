package gateway

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	commonv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/common/v1"
)

const (
	defaultPageSize = 50
	maxPageSize     = 200
)

func pageSize(page *commonv1.PageRequest) int {
	if page == nil || page.PageSize == 0 {
		return defaultPageSize
	}
	if page.PageSize > maxPageSize {
		return maxPageSize
	}

	return int(page.PageSize)
}

func offsetToken(prefix string, offset int) string {
	if offset <= 0 {
		return ""
	}

	raw := fmt.Sprintf("%s:%d", prefix, offset)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeOffset(page *commonv1.PageRequest, prefix string) (int, error) {
	if page == nil || strings.TrimSpace(page.PageToken) == "" {
		return 0, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(page.PageToken)
	if err != nil {
		return 0, fmt.Errorf("decode page token: %w", err)
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 || parts[0] != prefix {
		return 0, fmt.Errorf("decode page token: unexpected token prefix")
	}

	offset, err := strconv.Atoi(parts[1])
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("decode page token: invalid offset")
	}

	return offset, nil
}

func sequenceToken(prefix string, sequence uint64) string {
	if sequence == 0 {
		return ""
	}

	raw := fmt.Sprintf("%s:%d", prefix, sequence)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeSequence(page *commonv1.PageRequest, prefix string) (uint64, error) {
	if page == nil || strings.TrimSpace(page.PageToken) == "" {
		return 0, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(page.PageToken)
	if err != nil {
		return 0, fmt.Errorf("decode page token: %w", err)
	}
	parts := strings.SplitN(string(raw), ":", 2)
	if len(parts) != 2 || parts[0] != prefix {
		return 0, fmt.Errorf("decode page token: unexpected token prefix")
	}

	sequence, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("decode page token: invalid sequence")
	}

	return sequence, nil
}
