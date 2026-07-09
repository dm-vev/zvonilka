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
	id       string
	endpoint string
	runtime  nodeRuntime
	mu       sync.Mutex
	health   nodeHealth
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

// MigrateSession performs a controlled session cutover onto a different healthy node when possible.
func (c *Cluster) MigrateSession(ctx context.Context, callRow domaincall.Call) (domaincall.RuntimeSession, error) {
	if c == nil {
		return domaincall.RuntimeSession{}, domaincall.ErrInvalidInput
	}
	callID := strings.TrimSpace(callRow.ID)
	currentSessionID := strings.TrimSpace(callRow.ActiveSessionID)
	if callID == "" || currentSessionID == "" {
		return domaincall.RuntimeSession{}, domaincall.ErrInvalidInput
	}

	sourceNode, err := c.nodeForSession(currentSessionID)
	if err != nil {
		return domaincall.RuntimeSession{}, err
	}
	targetNode, err := c.migrationTargetNode(ctx, sourceNode.id, callID)
	if err != nil {
		return domaincall.RuntimeSession{}, err
	}
	if targetNode.id == sourceNode.id {
		return domaincall.RuntimeSession{}, domaincall.ErrConflict
	}

	var snapshot sessionSnapshot
	snapshot, err = sourceNode.runtime.ExportSessionSnapshot(ctx, currentSessionID)
	if err != nil {
		if saveErr := c.restoreReplicatedSnapshot(ctx, targetNode, callID); saveErr != nil {
			return domaincall.RuntimeSession{}, err
		}
	} else if saveErr := targetNode.runtime.SaveReplica(ctx, snapshot); saveErr != nil {
		return domaincall.RuntimeSession{}, saveErr
	}

	replacement := callRow
	replacement.ActiveSessionID = prefixedSessionID(targetNode.id, callID)
	session, err := targetNode.runtime.EnsureSession(ctx, replacement)
	if err != nil {
		return domaincall.RuntimeSession{}, err
	}
	if restoreErr := targetNode.runtime.RestoreReplica(ctx, callID, session.SessionID); restoreErr != nil && !errors.Is(restoreErr, domaincall.ErrNotFound) {
		return domaincall.RuntimeSession{}, restoreErr
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

// SessionState returns the current stable runtime placement for one call.
func (c *Cluster) SessionState(ctx context.Context, callRow domaincall.Call) (domaincall.RuntimeState, error) {
	state := domaincall.RuntimeState{
		CallID:         strings.TrimSpace(callRow.ID),
		ConversationID: strings.TrimSpace(callRow.ConversationID),
		SessionID:      strings.TrimSpace(callRow.ActiveSessionID),
		Active:         callRow.State == domaincall.StateActive && strings.TrimSpace(callRow.ActiveSessionID) != "",
		ObservedAt:     c.currentTime(),
	}
	if !state.Active {
		return state, nil
	}

	node, err := c.nodeForSession(state.SessionID)
	if err != nil {
		return domaincall.RuntimeState{}, err
	}

	state.NodeID = node.id
	state.RuntimeEndpoint = strings.TrimSpace(node.endpoint)
	healthy, err := c.nodeHealthy(ctx, node)
	if err == nil {
		state.Healthy = healthy
	}
	state.ConfiguredReplicaNodeIDs = c.configuredReplicaNodeIDs(node.id)
	state.HealthyMigrationTargetNodeIDs = c.healthyMigrationTargetNodeIDs(ctx, node.id)

	return domaincall.CloneRuntimeState(state), nil
}

// SessionSnapshot returns one exported stable runtime snapshot for the active session.
func (c *Cluster) SessionSnapshot(ctx context.Context, callRow domaincall.Call) (domaincall.RuntimeSnapshot, error) {
	if c == nil {
		return domaincall.RuntimeSnapshot{}, domaincall.ErrInvalidInput
	}

	sessionID := strings.TrimSpace(callRow.ActiveSessionID)
	if callRow.State != domaincall.StateActive || sessionID == "" {
		return domaincall.RuntimeSnapshot{}, domaincall.ErrConflict
	}

	node, err := c.nodeForSession(sessionID)
	if err != nil {
		return domaincall.RuntimeSnapshot{}, err
	}
	snapshot, err := node.runtime.ExportSessionSnapshot(ctx, sessionID)
	if err != nil {
		return domaincall.RuntimeSnapshot{}, err
	}

	return domaincall.CloneRuntimeSnapshot(domaincall.RuntimeSnapshot{
		CallID:         snapshot.CallID,
		ConversationID: snapshot.Conversation,
		SessionID:      sessionID,
		NodeID:         node.id,
		ObservedAt:     c.currentTime(),
		Participants:   runtimeSnapshotParticipants(snapshot.Participants),
	}), nil
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

func (c *Cluster) currentTime() time.Time {
	if c == nil || c.now == nil {
		return time.Now().UTC()
	}

	return c.now().UTC()
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

func (c *Cluster) migrationTargetNode(ctx context.Context, sourceNodeID string, callID string) (*clusterNode, error) {
	healthyNodes, err := c.healthyNodes(ctx)
	if err != nil {
		return nil, err
	}

	filtered := make([]*clusterNode, 0, len(healthyNodes))
	for _, node := range healthyNodes {
		if node == nil || node.id == strings.TrimSpace(sourceNodeID) {
			continue
		}
		filtered = append(filtered, node)
	}
	if len(filtered) == 0 {
		return c.byID[strings.TrimSpace(sourceNodeID)], nil
	}

	index := rendezvousIndex(callID, len(filtered))
	return filtered[index], nil
}

func (c *Cluster) restoreReplicatedSnapshot(ctx context.Context, target *clusterNode, callID string) error {
	if c == nil || target == nil || target.runtime == nil {
		return domaincall.ErrInvalidInput
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return domaincall.ErrInvalidInput
	}

	for _, node := range c.nodes {
		if node == nil || node.runtime == nil || node.id == target.id {
			continue
		}
		snapshot, err := node.runtime.ExportSessionSnapshot(ctx, prefixedSessionID(node.id, callID))
		if err != nil {
			continue
		}
		return target.runtime.SaveReplica(ctx, snapshot)
	}

	return domaincall.ErrNotFound
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
				id:       id,
				endpoint: strings.TrimSpace(cfg.PublicEndpoint),
				runtime:  local,
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
			id:       node.ID,
			endpoint: strings.TrimSpace(node.Endpoint),
			runtime:  runtime,
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

func (c *Cluster) configuredReplicaNodeIDs(primaryNodeID string) []string {
	result := make([]string, 0, len(c.nodes))
	for _, node := range c.nodes {
		if node == nil || node.id == strings.TrimSpace(primaryNodeID) {
			continue
		}
		result = append(result, node.id)
	}
	if len(result) == 0 {
		return nil
	}

	return result
}

func (c *Cluster) healthyMigrationTargetNodeIDs(ctx context.Context, primaryNodeID string) []string {
	result := make([]string, 0, len(c.nodes))
	for _, node := range c.nodes {
		if node == nil || node.id == strings.TrimSpace(primaryNodeID) {
			continue
		}
		healthy, err := c.nodeHealthy(ctx, node)
		if err != nil || !healthy {
			continue
		}
		result = append(result, node.id)
	}
	if len(result) == 0 {
		return nil
	}

	return result
}

func runtimeSnapshotParticipants(values []snapshotParticipant) []domaincall.RuntimeSnapshotParticipant {
	if len(values) == 0 {
		return nil
	}

	result := make([]domaincall.RuntimeSnapshotParticipant, 0, len(values))
	for _, value := range values {
		result = append(result, domaincall.RuntimeSnapshotParticipant{
			AccountID: value.AccountID,
			DeviceID:  value.DeviceID,
			WithVideo: value.WithVideo,
			Media: domaincall.MediaState{
				AudioMuted:         value.Media.AudioMuted,
				VideoMuted:         value.Media.VideoMuted,
				CameraEnabled:      value.Media.CameraEnabled,
				ScreenShareEnabled: value.Media.ScreenShareEnabled,
			},
			Transport: cloneTransportStats(value.Transport),
			Relay:     runtimeRelayTracks(value.Relay),
		})
	}

	return result
}

func runtimeRelayTracks(values []snapshotRelayTrack) []domaincall.RuntimeRelayTrack {
	if len(values) == 0 {
		return nil
	}

	result := make([]domaincall.RuntimeRelayTrack, 0, len(values))
	for _, value := range values {
		result = append(result, domaincall.RuntimeRelayTrack{
			SourceAccountID: value.SourceAccountID,
			SourceDeviceID:  value.SourceDeviceID,
			TrackID:         value.TrackID,
			StreamID:        value.StreamID,
			Kind:            value.Kind,
			ScreenShare:     value.ScreenShare,
			CodecMimeType:   value.CodecMimeType,
			CodecClockRate:  value.CodecClockRate,
			CodecChannels:   value.CodecChannels,
		})
	}

	return result
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
