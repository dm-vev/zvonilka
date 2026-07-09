package federationmeshtastic

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
	transport "github.com/dm-vev/zvonilka/internal/platform/federation/transport"
)

type settings struct {
	PeerServerName string
	LinkName       string
	PollInterval   time.Duration
	BatchSize      int
}

type worker struct {
	bridge   bridgeClient
	adapter  transport.Adapter
	settings settings
}

func newWorker(bridge bridgeClient, adapter transport.Adapter, settings settings) (*worker, error) {
	settings.PeerServerName = strings.TrimSpace(strings.ToLower(settings.PeerServerName))
	settings.LinkName = strings.TrimSpace(strings.ToLower(settings.LinkName))
	if bridge == nil || adapter == nil || settings.PeerServerName == "" || settings.LinkName == "" {
		return nil, errInvalidInput
	}
	if settings.PollInterval <= 0 {
		settings.PollInterval = 3 * time.Second
	}
	if settings.BatchSize <= 0 {
		settings.BatchSize = 16
	}

	return &worker{
		bridge:   bridge,
		adapter:  adapter,
		settings: settings,
	}, nil
}

func (w *worker) Run(ctx context.Context, logger *slog.Logger) error {
	if ctx == nil || logger == nil {
		return errInvalidInput
	}

	ticker := time.NewTicker(w.settings.PollInterval)
	defer ticker.Stop()

	for {
		if err := w.processOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.ErrorContext(ctx, "process meshtastic bridge batch", "err", err)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (w *worker) processOnce(ctx context.Context) error {
	if ctx == nil || w == nil || w.bridge == nil || w.adapter == nil {
		return errInvalidInput
	}

	pulled, err := w.bridge.PullFragments(
		ctx,
		w.settings.PeerServerName,
		w.settings.LinkName,
		w.settings.BatchSize,
	)
	if err != nil {
		return err
	}
	if pulled != nil && len(pulled.GetFragments()) > 0 {
		sentIDs, err := w.adapter.Send(
			ctx,
			w.settings.PeerServerName,
			w.settings.LinkName,
			pulled.GetLink(),
			pulled.GetFragments(),
		)
		if err != nil {
			return err
		}
		if len(sentIDs) > 0 {
			if _, err := w.bridge.AcknowledgeFragments(
				ctx,
				w.settings.PeerServerName,
				w.settings.LinkName,
				sentIDs,
				pulled.GetLeaseToken(),
			); err != nil {
				return err
			}
		}
	}

	received, err := w.adapter.Receive(ctx, w.settings.BatchSize)
	if err != nil {
		return err
	}
	if len(received) == 0 {
		return nil
	}

	grouped := make(map[string][]*federationv1.BundleFragment, len(received))
	for _, item := range received {
		peerServerName := strings.TrimSpace(strings.ToLower(item.PeerServerName))
		if peerServerName == "" {
			peerServerName = w.settings.PeerServerName
		}
		linkName := strings.TrimSpace(strings.ToLower(item.LinkName))
		if linkName == "" {
			linkName = w.settings.LinkName
		}
		if item.Fragment == nil || peerServerName == "" || linkName == "" {
			continue
		}

		key := peerServerName + "\x00" + linkName
		grouped[key] = append(grouped[key], cloneFragment(item.Fragment))
	}

	for key, fragments := range grouped {
		peerServerName, linkName, _ := strings.Cut(key, "\x00")
		if _, err := w.bridge.SubmitFragments(ctx, peerServerName, linkName, fragments); err != nil {
			return err
		}
	}

	return nil
}

func (w *worker) ProcessOnceForTests(ctx context.Context) error {
	return w.processOnce(ctx)
}

func (w *worker) Close() error {
	var closeErr error
	if w != nil && w.adapter != nil {
		closeErr = errors.Join(closeErr, w.adapter.Close())
	}
	if w != nil && w.bridge != nil {
		closeErr = errors.Join(closeErr, w.bridge.Close())
	}
	return closeErr
}

func cloneFragment(fragment *federationv1.BundleFragment) *federationv1.BundleFragment {
	if fragment == nil {
		return nil
	}

	copy := *fragment
	copy.Payload = append([]byte(nil), fragment.GetPayload()...)
	return &copy
}
