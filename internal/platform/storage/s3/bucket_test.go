package s3

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestEnsureBucketCreatesAfterHeadAccessDenied(t *testing.T) {
	t.Parallel()

	client := &bucketClientStub{
		headErrs: []error{
			apiError{code: "AccessDenied"},
			nil,
		},
	}

	if err := ensureBucket(context.Background(), client, "media", "us-east-1"); err != nil {
		t.Fatalf("ensure bucket: %v", err)
	}
	if client.createCalls != 1 {
		t.Fatalf("expected one create call, got %d", client.createCalls)
	}
	if client.headCalls != 2 {
		t.Fatalf("expected two head calls, got %d", client.headCalls)
	}
}

func TestEnsureBucketTreatsAlreadyOwnedAsSuccess(t *testing.T) {
	t.Parallel()

	client := &bucketClientStub{
		headErrs: []error{
			apiError{code: "AccessDenied"},
		},
		createErrs: []error{
			&s3types.BucketAlreadyOwnedByYou{},
		},
	}

	if err := ensureBucket(context.Background(), client, "media", "us-east-1"); err != nil {
		t.Fatalf("ensure bucket: %v", err)
	}
	if client.createCalls != 1 {
		t.Fatalf("expected one create call, got %d", client.createCalls)
	}
}

func TestEnsureBucketRetriesBucketAlreadyExistsUntilVisible(t *testing.T) {
	t.Parallel()

	client := &bucketClientStub{
		headErrs: []error{
			apiError{code: "AccessDenied"},
			nil,
		},
		createErrs: []error{
			&s3types.BucketAlreadyExists{},
		},
	}

	if err := ensureBucket(context.Background(), client, "media", "us-east-1"); err != nil {
		t.Fatalf("ensure bucket: %v", err)
	}
	if client.createCalls != 1 {
		t.Fatalf("expected one create call, got %d", client.createCalls)
	}
	if client.headCalls != 2 {
		t.Fatalf("expected two head calls, got %d", client.headCalls)
	}
}

type bucketClientStub struct {
	headErrs   []error
	createErrs []error

	headCalls   int
	createCalls int
}

func (s *bucketClientStub) HeadBucket(context.Context, *s3.HeadBucketInput, ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	s.headCalls++
	if idx := s.headCalls - 1; idx < len(s.headErrs) && s.headErrs[idx] != nil {
		return nil, s.headErrs[idx]
	}

	return &s3.HeadBucketOutput{}, nil
}

func (s *bucketClientStub) CreateBucket(context.Context, *s3.CreateBucketInput, ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	s.createCalls++
	if idx := s.createCalls - 1; idx < len(s.createErrs) && s.createErrs[idx] != nil {
		return nil, s.createErrs[idx]
	}

	return &s3.CreateBucketOutput{}, nil
}
