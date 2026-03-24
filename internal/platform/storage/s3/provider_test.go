package s3

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"

	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
)

func TestNormalizeEndpointAddsScheme(t *testing.T) {
	t.Parallel()

	got, err := normalizeEndpoint("127.0.0.1:9000", false)
	if err != nil {
		t.Fatalf("normalize endpoint: %v", err)
	}
	if got != "http://127.0.0.1:9000" {
		t.Fatalf("unexpected endpoint: %s", got)
	}

	got, err = normalizeEndpoint("s4.example.invalid", true)
	if err != nil {
		t.Fatalf("normalize endpoint with ssl: %v", err)
	}
	if got != "https://s4.example.invalid" {
		t.Fatalf("unexpected ssl endpoint: %s", got)
	}
}

func TestNormalizeEndpointRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	for _, tc := range []string{
		"",
		"http://",
		"ftp://s4.example.invalid",
		"http://s4.example.invalid/api",
		"https://s4.example.invalid?query=1",
	} {
		if _, err := normalizeEndpoint(tc, false); !errors.Is(err, domainstorage.ErrInvalidInput) {
			t.Fatalf("expected invalid input for %q, got %v", tc, err)
		}
	}
}

func TestMapObjectErrorMapsTerminalAccessDenied(t *testing.T) {
	t.Parallel()

	for _, apiErr := range []apiError{
		{code: "AccessDenied"},
		{code: "Forbidden"},
		{code: "Unauthorized"},
	} {
		mapped := mapObjectError(apiErr)
		if !errors.Is(mapped, domainstorage.ErrForbidden) {
			t.Fatalf("expected forbidden mapping for %q, got %v", apiErr.code, mapped)
		}
	}
}

func TestMapObjectErrorPreservesCancellation(t *testing.T) {
	t.Parallel()

	if !errors.Is(mapObjectError(context.Canceled), context.Canceled) {
		t.Fatal("expected cancellation sentinel to survive mapping")
	}
	if !errors.Is(mapObjectError(context.DeadlineExceeded), context.DeadlineExceeded) {
		t.Fatal("expected deadline sentinel to survive mapping")
	}
}

func TestToPresignedRequestCopiesBucketKeyAndHeaders(t *testing.T) {
	t.Parallel()

	request := &v4.PresignedHTTPRequest{
		URL:    "https://s4.example.invalid/media/test/object?X-Amz-Signature=abc",
		Method: http.MethodPut,
		SignedHeader: http.Header{
			"Content-Type":        []string{"text/plain"},
			"Content-Disposition": []string{"attachment"},
			"X-Amz-Meta-Trace":    []string{"a", "b"},
		},
	}

	got := toPresignedRequest(request, time.Minute, "zvonilka-media", "media/test/object")
	if got.URL != request.URL {
		t.Fatalf("unexpected url: %s", got.URL)
	}
	if got.Method != request.Method {
		t.Fatalf("unexpected method: %s", got.Method)
	}
	if got.Bucket != "zvonilka-media" {
		t.Fatalf("unexpected bucket: %s", got.Bucket)
	}
	if got.ObjectKey != "media/test/object" {
		t.Fatalf("unexpected object key: %s", got.ObjectKey)
	}
	if got.Headers["X-Amz-Meta-Trace"] != "a,b" {
		t.Fatalf("unexpected headers: %#v", got.Headers)
	}
}

func TestToPresignedRequestHandlesNilRequest(t *testing.T) {
	t.Parallel()

	got := toPresignedRequest(nil, time.Minute, "zvonilka-media", "media/test/object")
	if got.Bucket != "zvonilka-media" || got.ObjectKey != "media/test/object" {
		t.Fatalf("unexpected request projection: %+v", got)
	}
	if got.URL != "" || got.Method != "" {
		t.Fatalf("unexpected fallback values: %+v", got)
	}
}

func TestToPresignedRequestUsesSigningTimestamp(t *testing.T) {
	t.Parallel()

	request := &v4.PresignedHTTPRequest{
		URL:    "https://s4.example.invalid/media/test/object?X-Amz-Date=20260324T120000Z",
		Method: http.MethodGet,
	}

	got := toPresignedRequest(request, 2*time.Minute, "zvonilka-media", "media/test/object")
	want := time.Date(2026, time.March, 24, 12, 2, 0, 0, time.UTC)
	if !got.ExpiresAt.Equal(want) {
		t.Fatalf("expected expires at %s, got %s", want, got.ExpiresAt)
	}
}

func TestProviderNilAccessorsAreSafe(t *testing.T) {
	t.Parallel()

	var provider *Provider
	if provider.Name() != "" {
		t.Fatal("expected empty name from nil provider")
	}
	if provider.Kind() != domainstorage.KindUnspecified {
		t.Fatalf("expected unspecified kind from nil provider, got %s", provider.Kind())
	}
	if provider.Purpose() != domainstorage.PurposeUnspecified {
		t.Fatalf("expected unspecified purpose from nil provider, got %s", provider.Purpose())
	}
	if provider.Capabilities() != 0 {
		t.Fatalf("expected no capabilities from nil provider, got %v", provider.Capabilities())
	}
	if provider.Bucket() != "" {
		t.Fatal("expected empty bucket from nil provider")
	}
}

func TestProviderRejectsNilContext(t *testing.T) {
	t.Parallel()

	provider := &Provider{}

	if _, err := provider.PutObject(nil, "key", bytes.NewReader(nil), 0, domainstorage.PutObjectOptions{}); !errors.Is(err, domainstorage.ErrInvalidInput) {
		t.Fatalf("expected invalid input for nil context put, got %v", err)
	}
	if _, _, err := provider.GetObject(nil, "key"); !errors.Is(err, domainstorage.ErrInvalidInput) {
		t.Fatalf("expected invalid input for nil context get, got %v", err)
	}
	if _, err := provider.HeadObject(nil, "key"); !errors.Is(err, domainstorage.ErrInvalidInput) {
		t.Fatalf("expected invalid input for nil context head, got %v", err)
	}
	if err := provider.DeleteObject(nil, "key"); !errors.Is(err, domainstorage.ErrInvalidInput) {
		t.Fatalf("expected invalid input for nil context delete, got %v", err)
	}
	if _, err := provider.PresignPutObject(nil, "key", time.Minute, domainstorage.PutObjectOptions{}); !errors.Is(err, domainstorage.ErrInvalidInput) {
		t.Fatalf("expected invalid input for nil context presign put, got %v", err)
	}
	if _, err := provider.PresignGetObject(nil, "key", time.Minute); !errors.Is(err, domainstorage.ErrInvalidInput) {
		t.Fatalf("expected invalid input for nil context presign get, got %v", err)
	}
}

func TestProviderRejectsNilBody(t *testing.T) {
	t.Parallel()

	provider := &Provider{client: &s3.Client{}}
	if _, err := provider.PutObject(context.Background(), "key", nil, 0, domainstorage.PutObjectOptions{}); !errors.Is(err, domainstorage.ErrInvalidInput) {
		t.Fatalf("expected invalid input for nil body, got %v", err)
	}
}

func TestMapBucketCreateErrorMapsOwnershipStates(t *testing.T) {
	t.Parallel()

	if mapped := mapBucketCreateError(&types.BucketAlreadyOwnedByYou{}); mapped != nil {
		t.Fatalf("expected owned bucket to be treated as success, got %v", mapped)
	}
	if mapped := mapBucketCreateError(&types.BucketAlreadyExists{}); !errors.Is(mapped, domainstorage.ErrConflict) {
		t.Fatalf("expected existing bucket conflict, got %v", mapped)
	}
}

type apiError struct {
	code string
}

func (e apiError) Error() string {
	return e.code
}

func (e apiError) ErrorCode() string {
	return e.code
}

func (e apiError) ErrorMessage() string {
	return e.code
}

func (e apiError) ErrorFault() smithy.ErrorFault {
	return smithy.FaultClient
}
