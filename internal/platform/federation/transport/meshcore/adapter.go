package meshcore

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
	meshtastictransport "github.com/dm-vev/zvonilka/internal/platform/federation/transport/meshtastic"
)

var errInvalidConfig = errors.New("invalid meshcore config")

// Config defines how the MeshCore adapter talks to the local helper script.
type Config struct {
	InterfaceKind    string
	Device           string
	HelperPython     string
	HelperScriptPath string
	ReceiveTimeout   time.Duration
	TextPrefix       string
	Destination      string
}

type commandRunner interface {
	CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// Adapter exchanges federation fragments via the MeshCore Python helper.
type Adapter struct {
	cfg    Config
	runner commandRunner
}

// New constructs a MeshCore adapter backed by the helper script.
func New(cfg Config) (*Adapter, error) {
	cfg.InterfaceKind = strings.TrimSpace(strings.ToLower(cfg.InterfaceKind))
	cfg.Device = strings.TrimSpace(cfg.Device)
	cfg.HelperPython = strings.TrimSpace(cfg.HelperPython)
	cfg.HelperScriptPath = strings.TrimSpace(cfg.HelperScriptPath)
	cfg.TextPrefix = strings.TrimSpace(cfg.TextPrefix)
	cfg.Destination = strings.TrimSpace(cfg.Destination)
	if cfg.InterfaceKind == "" || cfg.Device == "" || cfg.HelperPython == "" || cfg.HelperScriptPath == "" || cfg.TextPrefix == "" || cfg.Destination == "" {
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

// Send publishes one batch of fragments through the local MeshCore helper.
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

		text, err := meshtastictransport.EncodeTextEnvelope(peerServerName, linkName, fragment, a.cfg.TextPrefix)
		if err != nil {
			return nil, err
		}
		if _, err := a.run(ctx, "send", "--destination", a.cfg.Destination, "--text", text); err != nil {
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

		fragment, err := meshtastictransport.DecodeTextEnvelope(line, a.cfg.TextPrefix)
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
		return nil, fmt.Errorf("run meshcore helper %s: %w: %s", command, err, strings.TrimSpace(string(output)))
	}

	return output, nil
}

var _ transport.Adapter = (*Adapter)(nil)
