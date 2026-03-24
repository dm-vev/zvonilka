package controlplane

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	domainstorage "github.com/dm-vev/zvonilka/internal/domain/storage"
)

func TestAppCloseUsesCleanupContextWhenRuntimeContextCanceled(t *testing.T) {
	t.Parallel()

	provider := &cancelAwareProvider{name: "primary"}
	catalog, err := domainstorage.NewCatalog(provider)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = (&app{catalog: catalog}).close(ctx)
	require.NoError(t, err)
	require.True(t, provider.closed)
	require.NoError(t, provider.closeCtxErr)
}

func TestFinalizeRunKeepsRunErrorWhenRuntimeContextCanceled(t *testing.T) {
	t.Parallel()

	runErr := context.DeadlineExceeded
	provider := &cancelAwareProvider{name: "primary"}
	catalog, err := domainstorage.NewCatalog(provider)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got := finalizeRun(ctx, &app{catalog: catalog}, runErr)
	require.ErrorIs(t, got, runErr)
	require.False(t, provider.closeCtxWasCanceled)
	require.True(t, provider.closed)
}

func TestCleanupContextUsesExplicitFallbackAfterCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	cancel()

	cleanupCtx, cleanupCancel := cleanupContext(ctx, 5*time.Minute)
	defer cleanupCancel()

	deadline, ok := cleanupCtx.Deadline()
	require.True(t, ok)
	require.NoError(t, cleanupCtx.Err())
	require.WithinDuration(t, time.Now().Add(5*time.Minute), deadline, 2*time.Second)
}

func TestCleanupContextUsesDefaultBudgetWhenFallbackMissing(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	cancel()

	cleanupCtx, cleanupCancel := cleanupContext(ctx)
	defer cleanupCancel()

	deadline, ok := cleanupCtx.Deadline()
	require.True(t, ok)
	require.NoError(t, cleanupCtx.Err())
	require.WithinDuration(t, time.Now().Add(30*time.Second), deadline, 2*time.Second)
}

type cancelAwareProvider struct {
	name                string
	closed              bool
	closeCtxWasCanceled bool
	closeCtxErr         error
}

func (p *cancelAwareProvider) Name() string {
	return p.name
}

func (p *cancelAwareProvider) Kind() domainstorage.Kind {
	return domainstorage.KindCustom
}

func (p *cancelAwareProvider) Purpose() domainstorage.Purpose {
	return domainstorage.PurposeCustom
}

func (p *cancelAwareProvider) Capabilities() domainstorage.Capability {
	return domainstorage.CapabilityTransactions
}

func (p *cancelAwareProvider) Close(ctx context.Context) error {
	p.closed = true
	p.closeCtxErr = ctx.Err()
	p.closeCtxWasCanceled = p.closeCtxErr != nil
	if p.closeCtxErr != nil {
		return p.closeCtxErr
	}

	return nil
}
