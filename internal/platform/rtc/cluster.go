package rtc

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

// Cluster routes media sessions across logical in-server RTC nodes.
type Cluster struct {
	nodes []*clusterNode
	byID  map[string]*clusterNode
}

type clusterNode struct {
	id      string
	runtime nodeRuntime
}

type nodeRuntime interface {
	domaincall.Runtime
	Close(context.Context) error
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
		nodes: nodes,
		byID:  byID,
	}, nil
}

// EnsureSession creates or resolves the active media session for a call on its assigned node.
func (c *Cluster) EnsureSession(ctx context.Context, callRow domaincall.Call) (domaincall.RuntimeSession, error) {
	node, err := c.nodeForCall(callRow)
	if err != nil {
		return domaincall.RuntimeSession{}, err
	}

	if strings.TrimSpace(callRow.ActiveSessionID) == "" {
		callRow.ActiveSessionID = prefixedSessionID(node.id, callRow.ID)
	}

	return node.runtime.EnsureSession(ctx, callRow)
}

func (c *Cluster) JoinSession(ctx context.Context, sessionID string, participant domaincall.RuntimeParticipant) (domaincall.RuntimeJoin, error) {
	node, err := c.nodeForSession(sessionID)
	if err != nil {
		return domaincall.RuntimeJoin{}, err
	}

	return node.runtime.JoinSession(ctx, sessionID, participant)
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

	return node.runtime.UpdateParticipant(ctx, sessionID, participant)
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

	return node.runtime.LeaveSession(ctx, sessionID, accountID, deviceID)
}

func (c *Cluster) CloseSession(ctx context.Context, sessionID string) error {
	node, err := c.nodeForSession(sessionID)
	if err != nil {
		return err
	}

	return node.runtime.CloseSession(ctx, sessionID)
}

func (c *Cluster) nodeForCall(callRow domaincall.Call) (*clusterNode, error) {
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

	index := rendezvousIndex(callID, len(c.nodes))
	return c.nodes[index], nil
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
