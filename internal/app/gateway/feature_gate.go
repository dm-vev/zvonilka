package gateway

import (
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func featureDisabledError(feature string) error {
	return status.Error(codes.FailedPrecondition, fmt.Sprintf("%s feature is disabled", feature))
}
