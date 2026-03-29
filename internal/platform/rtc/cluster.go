package rtc

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

// Cluster routes media sessions across logical in-server RTC nodes.
type Cluster struct {
	nodes         []*clusterNode
	byID          map[string]*clusterNode
	healthTTL     time.Duration
	healthTimeout time.Duration
	now           func() time.Time
}

type clusterNode struct {
	id      string
	runtime nodeRuntime
	mu      sync.Mutex
	health  nodeHealth
}

type nodeRuntime interface {
	domaincall.Runtime
	ExportSessionSnapshot(context.Context, string) (sessionSnapshot, error)
	SaveReplica(context.Context, sessionSnapshot) error
	RestoreReplica(context.Context, string, string) error
	Close(context.Context) error
}

type healthRuntime interface {
	Healthy(context.Context) error
}

type nodeHealth struct {
	checkedAt time.Time
	healthy   bool
}

// NewCluster constructs a node-aware RTC runtime from the current RTC config.
func NewCluster(cfg domaincall.RTCConfig, local *Manager) (*Cluster, error) {
	cfg = cfg.NormalizeForPlatform()

	nodes, err := buildClusterNodes(cfg, local)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*clusterNode, len(nodes))
	for _, node := range nodes {
		byID[node.id] = node
	}

	return &Cluster{
		nodes:         nodes,
		byID:          byID,
		healthTTL:     cfg.HealthTTL,
		healthTimeout: cfg.HealthTimeout,
		now:           func() time.Time { return time.Now().UTC() },
	}, nil
}

// EnsureSession creates or resolves the active media session for a call on its assigned node.
func (c *Cluster) EnsureSession(ctx context.Context, callRow domaincall.Call) (domaincall.RuntimeSession, error) {
	node, err := c.nodeForCall(ctx, callRow)
	if err != nil {
		return domaincall.RuntimeSession{}, err
	}

	creating := strings.TrimSpace(callRow.ActiveSessionID) == ""
	if strings.TrimSpace(callRow.ActiveSessionID) == "" {
		callRow.ActiveSessionID = prefixedSessionID(node.id, callRow.ID)
	}

	session, err := node.runtime.EnsureSession(ctx, callRow)
	if err != nil {
		return domaincall.RuntimeSession{}, err
	}
	if creating && strings.TrimSpace(callRow.ID) != "" {
		if restoreErr := node.runtime.RestoreReplica(ctx, callRow.ID, session.SessionID); restoreErr != nil && !errors.Is(restoreErr, domaincall.ErrNotFound) {
			return domaincall.RuntimeSession{}, restoreErr
		}
	}
	_ = c.replicateSession(ctx, session.SessionID)

	return session, nil
}

func (c *Cluster) JoinSession(ctx context.Context, sessionID string, participant domaincall.RuntimeParticipant) (domaincall.RuntimeJoin, error) {
	node, err := c.nodeForSession(sessionID)
	if err != nil {
		return domaincall.RuntimeJoin{}, err
	}

	join, err := node.runtime.JoinSession(ctx, sessionID, participant)
	if err != nil {
		return domaincall.RuntimeJoin{}, err
	}
	_ = c.replicateSession(ctx, sessionID)
	return join, nil
}

func (c *Cluster) PublishDescription(
	ctx context.Context,
	sessionID string,
	participant domaincall.RuntimeParticipant,
	description domaincall.SessionDescription,
) ([]domaincall.RuntimeSignal, error) {
	node, err := c.nodeForSession(sessionID)
	if err != nil {
		return nil, err
	}

	return node.runtime.PublishDescription(ctx, sessionID, participant, description)
}

func (c *Cluster) PublishCandidate(
	ctx context.Context,
	sessionID string,
	participant domaincall.RuntimeParticipant,
	candidate domaincall.Candidate,
) ([]domaincall.RuntimeSignal, error) {
	node, err := c.nodeForSession(sessionID)
	if err != nil {
		return nil, err
	}

	return node.runtime.PublishCandidate(ctx, sessionID, participant, candidate)
}

func (c *Cluster) UpdateParticipant(ctx context.Context, sessionID string, participant domaincall.RuntimeParticipant) error {
	node, err := c.nodeForSession(sessionID)
	if err != nil {
		return err
	}

	if err := node.runtime.UpdateParticipant(ctx, sessionID, participant); err != nil {
		return err
	}
	_ = c.replicateSession(ctx, sessionID)
	return nil
}

func (c *Cluster) AcknowledgeAdaptation(
	ctx context.Context,
	sessionID string,
	participant domaincall.RuntimeParticipant,
	adaptationRevision uint64,
	appliedProfile string,
) error {
	node, err := c.nodeForSession(sessionID)
	if err != nil {
		return err
	}

	return node.runtime.AcknowledgeAdaptation(ctx, sessionID, participant, adaptationRevision, appliedProfile)
}

func (c *Cluster) SessionStats(ctx context.Context, sessionID string) ([]domaincall.RuntimeStats, error) {
	node, err := c.nodeForSession(sessionID)
	if err != nil {
		return nil, err
	}

	return node.runtime.SessionStats(ctx, sessionID)
}

func (c *Cluster) LeaveSession(ctx context.Context, sessionID string, accountID string, deviceID string) error {
	node, err := c.nodeForSession(sessionID)
	if err != nil {
		return err
	}

	if err := node.runtime.LeaveSession(ctx, sessionID, accountID, deviceID); err != nil {
		return err
	}
	_ = c.replicateSession(ctx, sessionID)
	return nil
}

func (c *Cluster) CloseSession(ctx context.Context, sessionID string) error {
	node, err := c.nodeForSession(sessionID)
	if err != nil {
		return err
	}

	return node.runtime.CloseSession(ctx, sessionID)
}

func (c *Cluster) nodeForCall(ctx context.Context, callRow domaincall.Call) (*clusterNode, error) {
	if c == nil || len(c.nodes) == 0 {
		return nil, domaincall.ErrInvalidInput
	}

	sessionID := strings.TrimSpace(callRow.ActiveSessionID)
	if sessionID != "" {
		return c.nodeForSession(sessionID)
	}

	callID := strings.TrimSpace(callRow.ID)
	if callID == "" {
		return nil, domaincall.ErrInvalidInput
	}

	healthyNodes, err := c.healthyNodes(ctx)
	if err != nil {
		return nil, err
	}
	index := rendezvousIndex(callID, len(healthyNodes))
	return healthyNodes[index], nil
}

func (c *Cluster) nodeForSession(sessionID string) (*clusterNode, error) {
	if c == nil || len(c.nodes) == 0 {
		return nil, domaincall.ErrInvalidInput
	}

	nodeID := domaincall.NodeIDFromSessionID(sessionID)
	if nodeID == "" {
		return nil, domaincall.ErrInvalidInput
	}

	node := c.byID[nodeID]
	if node == nil {
		return nil, domaincall.ErrNotFound
	}

	return node, nil
}

func (c *Cluster) Close(ctx context.Context) error {
	if c == nil {
		return nil
	}

	var closeErr error
	for _, node := range c.nodes {
		if node == nil || node.runtime == nil {
			continue
		}
		if err := node.runtime.Close(ctx); err != nil {
			closeErr = errors.Join(closeErr, err)
		}
	}

	return closeErr
}

func (c *Cluster) replicateSession(ctx context.Context, sessionID string) error {
	node, err := c.nodeForSession(sessionID)
	if err != nil {
		return err
	}
	snapshot, err := node.runtime.ExportSessionSnapshot(ctx, sessionID)
	if err != nil {
		return err
	}

	for _, target := range c.nodes {
		if target == nil || target.runtime == nil || target.id == node.id {
			continue
		}
		if saveErr := target.runtime.SaveReplica(ctx, snapshot); saveErr != nil {
			continue
		}
	}

	return nil
}

func (c *Cluster) healthyNodes(ctx context.Context) ([]*clusterNode, error) {
	if c == nil || len(c.nodes) == 0 {
		return nil, domaincall.ErrInvalidInput
	}

	result := make([]*clusterNode, 0, len(c.nodes))
	for _, node := range c.nodes {
		healthy, err := c.nodeHealthy(ctx, node)
		if err != nil {
			continue
		}
		if healthy {
			result = append(result, node)
		}
	}
	if len(result) == 0 {
		return nil, domaincall.ErrNotFound
	}

	return result, nil
}

func (c *Cluster) nodeHealthy(ctx context.Context, node *clusterNode) (bool, error) {
	if c == nil || node == nil || node.runtime == nil {
		return false, domaincall.ErrInvalidInput
	}
	if _, ok := node.runtime.(healthRuntime); !ok {
		return true, nil
	}

	now := c.now()

	node.mu.Lock()
	if !node.health.checkedAt.IsZero() && now.Sub(node.health.checkedAt) < c.healthTTL {
		healthy := node.health.healthy
		node.mu.Unlock()
		return healthy, nil
	}
	node.mu.Unlock()

	probeCtx := ctx
	cancel := func() {}
	if c.healthTimeout > 0 {
		probeCtx, cancel = context.WithTimeout(ctx, c.healthTimeout)
	}
	defer cancel()

	err := node.runtime.(healthRuntime).Healthy(probeCtx)

	node.mu.Lock()
	node.health.checkedAt = now
	node.health.healthy = err == nil
	healthy := node.health.healthy
	node.mu.Unlock()

	if err != nil {
		return false, err
	}

	return healthy, nil
}

func buildClusterNodes(cfg domaincall.RTCConfig, local *Manager) ([]*clusterNode, error) {
	localID := strings.TrimSpace(cfg.NodeID)
	if len(cfg.Nodes) == 0 {
		id := localID
		if id == "" {
			id = "node-local"
		}
		if local == nil {
			return nil, domaincall.ErrInvalidInput
		}
		return []*clusterNode{
			{
				id:      id,
				runtime: local,
			},
		}, nil
	}

	nodes := append([]domaincall.RTCNode(nil), cfg.Nodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })

	ranges := splitPortRange(cfg.UDPPortMin, cfg.UDPPortMax, len(nodes))
	result := make([]*clusterNode, 0, len(nodes))
	for i, node := range nodes {
		var runtime nodeRuntime
		switch {
		case node.ID == localID:
			if local == nil {
				return nil, domaincall.ErrInvalidInput
			}
			runtime = local
		case strings.TrimSpace(node.ControlEndpoint) != "":
			client, err := newGRPCRuntimeClient(node.ControlEndpoint)
			if err != nil {
				return nil, fmt.Errorf("dial rtc node %s: %w", node.ID, err)
			}
			runtime = client
		default:
			runtime = newConfiguredManager(node.Endpoint, cfg.CredentialTTL, cfg.CandidateHost, ranges[i][0], ranges[i][1])
		}
		result = append(result, &clusterNode{
			id:      node.ID,
			runtime: runtime,
		})
	}

	return result, nil
}

func newConfiguredManager(endpoint string, ttl time.Duration, host string, portMin int, portMax int) *Manager {
	return NewManager(
		endpoint,
		ttl,
		WithCandidateHost(host),
		WithUDPPortRange(portMin, portMax),
	)
}

func (m *Manager) Close(context.Context) error {
	return nil
}

func splitPortRange(min int, max int, count int) [][2]int {
	if count <= 0 {
		return nil
	}
	if max < min {
		max = min
	}

	total := max - min + 1

	base := total / count
	rem := total % count
	ranges := make([][2]int, 0, count)
	start := min
	for i := 0; i < count; i++ {
		size := base
		if rem > 0 {
			size++
			rem--
		}
		if size <= 0 {
			size = 1
		}
		end := start + size - 1
		if end > max {
			end = max
		}
		if start > max {
			start = max
		}
		ranges = append(ranges, [2]int{start, end})
		if end < max {
			start = end + 1
		}
	}

	return ranges
}

func rendezvousIndex(key string, count int) int {
	if count <= 1 {
		return 0
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(strings.TrimSpace(key)))
	return int(hasher.Sum32() % uint32(count))
}

func prefixedSessionID(nodeID string, callID string) string {
	nodeID = strings.TrimSpace(nodeID)
	callID = strings.TrimSpace(callID)
	if nodeID == "" {
		return "rtc_" + callID
	}

	return fmt.Sprintf("%s:rtc_%s", nodeID, callID)
}
