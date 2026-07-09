package meshtastic

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	federationv1 "github.com/dm-vev/zvonilka/gen/proto/contracts/federation/v1"
	"github.com/dm-vev/zvonilka/internal/platform/federation/transport"
)

var (
	errInvalidConfig       = errors.New("invalid meshtastic config")
	errEnvelopePrefix      = errors.New("unsupported meshtastic frame prefix")
	errUnsupportedEnvelope = errors.New("unsupported meshtastic envelope version")
)

// Config defines how the Meshtastic adapter talks to the local helper script.
type Config struct {
	InterfaceKind    string
	Device           string
	HelperPython     string
	HelperScriptPath string
	ReceiveTimeout   time.Duration
	TextPrefix       string
}

type commandRunner interface {
	CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// Adapter exchanges federation fragments via the Meshtastic Python SDK helper.
type Adapter struct {
	cfg    Config
	runner commandRunner
}

// New constructs a Meshtastic adapter backed by the helper script.
func New(cfg Config) (*Adapter, error) {
	cfg.InterfaceKind = strings.TrimSpace(strings.ToLower(cfg.InterfaceKind))
	cfg.Device = strings.TrimSpace(cfg.Device)
	cfg.HelperPython = strings.TrimSpace(cfg.HelperPython)
	cfg.HelperScriptPath = strings.TrimSpace(cfg.HelperScriptPath)
	cfg.TextPrefix = strings.TrimSpace(cfg.TextPrefix)
	if cfg.InterfaceKind == "" || cfg.Device == "" || cfg.HelperPython == "" || cfg.HelperScriptPath == "" || cfg.TextPrefix == "" {
		return nil, errInvalidConfig
	}
	if cfg.ReceiveTimeout <= 0 {
		return nil, errInvalidConfig
	}

	return &Adapter{
		cfg:    cfg,
		runner: execRunner{},
	}, nil
}

func newWithRunner(cfg Config, runner commandRunner) (*Adapter, error) {
	adapter, err := New(cfg)
	if err != nil {
		return nil, err
	}
	if runner == nil {
		return nil, errInvalidConfig
	}
	adapter.runner = runner
	return adapter, nil
}

// Send publishes one batch of fragments through the local Meshtastic helper.
func (a *Adapter) Send(
	ctx context.Context,
	peerServerName string,
	linkName string,
	_ *federationv1.Link,
	fragments []*federationv1.BundleFragment,
) ([]string, error) {
	if a == nil || a.runner == nil {
		return nil, errInvalidConfig
	}

	sent := make([]string, 0, len(fragments))
	for _, fragment := range fragments {
		if fragment == nil {
			continue
		}

		text, err := EncodeTextEnvelope(peerServerName, linkName, fragment, a.cfg.TextPrefix)
		if err != nil {
			return nil, err
		}
		if _, err := a.run(ctx, "send", "--text", text); err != nil {
			return nil, err
		}
		sent = append(sent, fragment.GetFragmentId())
	}

	return sent, nil
}

// Receive polls the helper for inbound text frames and decodes them into federation fragments.
func (a *Adapter) Receive(ctx context.Context, limit int) ([]transport.ReceivedFragment, error) {
	if a == nil || a.runner == nil {
		return nil, errInvalidConfig
	}
	if limit <= 0 {
		return nil, nil
	}

	output, err := a.run(
		ctx,
		"listen",
		"--limit", strconv.Itoa(limit),
		"--timeout-seconds", strconv.FormatFloat(a.cfg.ReceiveTimeout.Seconds(), 'f', -1, 64),
		"--prefix", a.cfg.TextPrefix,
	)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	received := make([]transport.ReceivedFragment, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fragment, err := DecodeTextEnvelope(line, a.cfg.TextPrefix)
		if err != nil {
			return nil, err
		}
		received = append(received, fragment)
	}

	return received, nil
}

// Close releases adapter resources.
func (a *Adapter) Close() error {
	return nil
}

func (a *Adapter) run(ctx context.Context, command string, args ...string) ([]byte, error) {
	argv := []string{
		a.cfg.HelperScriptPath,
		command,
		"--interface-kind", a.cfg.InterfaceKind,
		"--device", a.cfg.Device,
	}
	argv = append(argv, args...)

	output, err := a.runner.CombinedOutput(ctx, a.cfg.HelperPython, argv...)
	if err != nil {
		return nil, fmt.Errorf("run meshtastic helper %s: %w: %s", command, err, strings.TrimSpace(string(output)))
	}

	return output, nil
}

var _ transport.Adapter = (*Adapter)(nil)
