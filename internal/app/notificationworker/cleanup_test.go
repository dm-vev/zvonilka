package notificationworker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCleanupContextIgnoresCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cleanupCtx, cleanupCancel := cleanupContext(ctx, 50*time.Millisecond)
	defer cleanupCancel()

	require.NoError(t, cleanupCtx.Err())
	deadline, ok := cleanupCtx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(50*time.Millisecond), deadline, 25*time.Millisecond)
}

func TestCleanupContextHonorsFallbackTimeout(t *testing.T) {
	t.Parallel()

	cleanupCtx, cleanupCancel := cleanupContext(context.Background(), 75*time.Millisecond)
	defer cleanupCancel()

	require.NoError(t, cleanupCtx.Err())
	deadline, ok := cleanupCtx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(75*time.Millisecond), deadline, 25*time.Millisecond)
}

func TestCleanupContextClampsToParentDeadline(t *testing.T) {
	t.Parallel()

	parent, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	cleanupCtx, cleanupCancel := cleanupContext(parent, 200*time.Millisecond)
	defer cleanupCancel()

	require.NoError(t, cleanupCtx.Err())
	deadline, ok := cleanupCtx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(40*time.Millisecond), deadline, 25*time.Millisecond)
}

func TestFinalizeRunUsesCleanupContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fake := &fakeBootstrap{}
	err := finalizeRun(ctx, &app{bootstrap: fake, cleanupTimeout: 60 * time.Millisecond}, nil)
	require.NoError(t, err)
	require.True(t, fake.closed)
	require.NoError(t, fake.closeCtxErr)
	deadline, ok := fake.closedCtx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(60*time.Millisecond), deadline, 25*time.Millisecond)
}

func TestCloseBootstrapUsesConfiguredTimeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fake := &fakeBootstrap{}
	err := closeBootstrap(ctx, fake, 90*time.Millisecond)
	require.NoError(t, err)
	require.True(t, fake.closed)
	require.NoError(t, fake.closeCtxErr)
	deadline, ok := fake.closedCtx.Deadline()
	require.True(t, ok)
	require.WithinDuration(t, time.Now().Add(90*time.Millisecond), deadline, 30*time.Millisecond)
}

type fakeBootstrap struct {
	closed      bool
	closedCtx   context.Context
	closeCtxErr error
	closeErr    error
}

func (f *fakeBootstrap) Close(ctx context.Context) error {
	f.closed = true
	f.closedCtx = ctx
	f.closeCtxErr = ctx.Err()
	return f.closeErr
}

var _ bootstrapCloser = (*fakeBootstrap)(nil)

func TestFinalizeRunReturnsCloseError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("close failed")
	err := finalizeRun(context.Background(), &app{bootstrap: &fakeBootstrap{closeErr: wantErr}}, nil)
	require.ErrorIs(t, err, wantErr)
}
