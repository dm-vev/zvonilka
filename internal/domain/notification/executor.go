package notification

import (
	"context"
	"errors"
)

// DeliveryTarget carries resolved routing metadata for one delivery attempt.
type DeliveryTarget struct {
	PushToken *PushToken
}

// DeliveryExecutor dispatches one leased delivery to an external transport.
type DeliveryExecutor interface {
	Deliver(ctx context.Context, delivery Delivery, target DeliveryTarget) error
}

type deliveryError struct {
	err       error
	permanent bool
}

func (e *deliveryError) Error() string {
	if e == nil || e.err == nil {
		return "notification delivery failed"
	}

	return e.err.Error()
}

func (e *deliveryError) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.err
}

// PermanentDeliveryError wraps an error that should fail the delivery without retrying.
func PermanentDeliveryError(err error) error {
	if err == nil {
		return nil
	}

	return &deliveryError{
		err:       err,
		permanent: true,
	}
}

func isPermanentDeliveryError(err error) bool {
	var deliveryErr *deliveryError
	if !errors.As(err, &deliveryErr) {
		return false
	}

	return deliveryErr.permanent
}
