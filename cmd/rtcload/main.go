package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	callruntimev1 "github.com/dm-vev/zvonilka/internal/genproto/callruntime/v1"
	"github.com/pion/webrtc/v4"
	"google.golang.org/grpc"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
	"github.com/dm-vev/zvonilka/internal/platform/rtc"
)

type result struct {
	Mode               string        `json:"mode"`
	Sessions           int           `json:"sessions"`
	Concurrency        int           `json:"concurrency"`
	WithVideo          bool          `json:"with_video"`
	Duration           time.Duration `json:"duration"`
	OpsPerSecond       float64       `json:"ops_per_second"`
	Failures           uint64        `json:"failures"`
	EnsureAvg          time.Duration `json:"ensure_avg"`
	JoinAvg            time.Duration `json:"join_avg"`
	NegotiateAvg       time.Duration `json:"negotiate_avg"`
	StatsAvg           time.Duration `json:"stats_avg"`
	MigrateAvg         time.Duration `json:"migrate_avg"`
	CloseAvg           time.Duration `json:"close_avg"`
	GoMaxProcs         int           `json:"gomaxprocs"`
	GoRoutines         int           `json:"goroutines"`
	EstimatedPeakPeers int           `json:"estimated_peak_peers"`
	RelayOnly          bool          `json:"relay_only"`
	TURNURL            string        `json:"turn_url,omitempty"`
}

type timings struct {
	ensureNanos    atomic.Int64
	joinNanos      atomic.Int64
	negotiateNanos atomic.Int64
	statsNanos     atomic.Int64
	migrateNanos   atomic.Int64
	closeNanos     atomic.Int64
}

func main() {
	var (
		mode        = flag.String("mode", "negotiate", "load mode: lifecycle|negotiate|cluster-lifecycle|cluster-migrate")
		sessions    = flag.Int("sessions", 100, "number of sessions to execute")
		concurrency = flag.Int("concurrency", 8, "number of concurrent workers")
		withVideo   = flag.Bool("with-video", true, "whether participants join with video")
		relayOnly   = flag.Bool("relay-only", false, "force client negotiation through TURN relay")
		turnURL     = flag.String("turn-url", "", "TURN URL used when relay-only is enabled")
		turnSecret  = flag.String("turn-secret", "", "TURN REST auth secret used when relay-only is enabled")
	)
	flag.Parse()

	if *sessions <= 0 {
		fatalf("sessions must be > 0")
	}
	if *concurrency <= 0 {
		fatalf("concurrency must be > 0")
	}
	if *mode != "lifecycle" && *mode != "negotiate" && *mode != "cluster-lifecycle" && *mode != "cluster-migrate" {
		fatalf("unsupported mode %q", *mode)
	}
	if *relayOnly && (trim(*turnURL) == "" || trim(*turnSecret) == "") {
		fatalf("relay-only mode requires -turn-url and -turn-secret")
	}

	manager := rtc.NewManager(
		"webrtc://gateway/calls",
		15*time.Minute,
		rtc.WithCandidateHost("127.0.0.1"),
		rtc.WithUDPPortRange(47000, 49000),
	)
	var harness *clusterHarness
	if *mode == "cluster-lifecycle" || *mode == "cluster-migrate" {
		var err error
		harness, err = newClusterHarness()
		if err != nil {
			fatalf("create cluster harness: %v", err)
		}
		defer func() {
			if err := harness.Close(context.Background()); err != nil {
				fatalf("close cluster harness: %v", err)
			}
		}()
	}
	ctx := context.Background()

	jobs := make(chan int)
	var wg sync.WaitGroup
	var failures atomic.Uint64
	var samples timings
	startedAt := time.Now()

	for range *concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				if err := runScenario(ctx, manager, harness, *mode, *withVideo, index, *relayOnly, *turnURL, *turnSecret, &samples); err != nil {
					failures.Add(1)
				}
			}
		}()
	}

	for i := range *sessions {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	elapsed := time.Since(startedAt)
	output := result{
		Mode:               *mode,
		Sessions:           *sessions,
		Concurrency:        *concurrency,
		WithVideo:          *withVideo,
		Duration:           elapsed,
		OpsPerSecond:       float64(*sessions) / elapsed.Seconds(),
		Failures:           failures.Load(),
		EnsureAvg:          averageDuration(samples.ensureNanos.Load(), *sessions),
		JoinAvg:            averageDuration(samples.joinNanos.Load(), *sessions*2),
		NegotiateAvg:       averageDuration(samples.negotiateNanos.Load(), *sessions),
		StatsAvg:           averageDuration(samples.statsNanos.Load(), *sessions),
		MigrateAvg:         averageDuration(samples.migrateNanos.Load(), *sessions),
		CloseAvg:           averageDuration(samples.closeNanos.Load(), *sessions),
		GoMaxProcs:         runtime.GOMAXPROCS(0),
		GoRoutines:         runtime.NumGoroutine(),
		EstimatedPeakPeers: *concurrency * 2,
		RelayOnly:          *relayOnly,
		TURNURL:            trim(*turnURL),
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		fatalf("encode result: %v", err)
	}
	if output.Failures > 0 {
		os.Exit(1)
	}
}

func runScenario(
	ctx context.Context,
	manager *rtc.Manager,
	harness *clusterHarness,
	mode string,
	withVideo bool,
	index int,
	relayOnly bool,
	turnURL string,
	turnSecret string,
	samples *timings,
) error {
	if mode == "cluster-lifecycle" || mode == "cluster-migrate" {
		return runClusterScenario(ctx, harness, mode, withVideo, index, samples)
	}

	callID := fmt.Sprintf("load-call-%d", index)

	ensureStart := time.Now()
	session, err := manager.EnsureSession(ctx, domaincall.Call{
		ID:             callID,
		ConversationID: "load-conv-" + callID,
	})
	if err != nil {
		return err
	}
	samples.ensureNanos.Add(time.Since(ensureStart).Nanoseconds())

	joinStart := time.Now()
	if _, err := manager.JoinSession(ctx, session.SessionID, domaincall.RuntimeParticipant{
		CallID:    callID,
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: withVideo,
	}); err != nil {
		return err
	}
	if _, err := manager.JoinSession(ctx, session.SessionID, domaincall.RuntimeParticipant{
		CallID:    callID,
		AccountID: "acc-b",
		DeviceID:  "dev-b",
		WithVideo: withVideo,
	}); err != nil {
		return err
	}
	samples.joinNanos.Add(time.Since(joinStart).Nanoseconds())

	if mode == "negotiate" {
		negotiateStart := time.Now()
		if err := negotiateOnce(ctx, manager, session.SessionID, callID, withVideo, relayOnly, turnURL, turnSecret); err != nil {
			return err
		}
		samples.negotiateNanos.Add(time.Since(negotiateStart).Nanoseconds())
	}

	statsStart := time.Now()
	if _, err := manager.SessionStats(ctx, session.SessionID); err != nil {
		return err
	}
	samples.statsNanos.Add(time.Since(statsStart).Nanoseconds())

	closeStart := time.Now()
	if err := manager.CloseSession(ctx, session.SessionID); err != nil {
		return err
	}
	samples.closeNanos.Add(time.Since(closeStart).Nanoseconds())

	return nil
}

func runClusterScenario(
	ctx context.Context,
	harness *clusterHarness,
	mode string,
	withVideo bool,
	index int,
	samples *timings,
) error {
	if harness == nil {
		return fmt.Errorf("missing cluster harness")
	}

	callID := fmt.Sprintf("cluster-load-call-%d", index)
	sessionID := "node-b:rtc_" + callID

	ensureStart := time.Now()
	session, err := harness.cluster.EnsureSession(ctx, domaincall.Call{
		ID:              callID,
		ConversationID:  "cluster-load-conv-" + callID,
		ActiveSessionID: sessionID,
	})
	if err != nil {
		return err
	}
	samples.ensureNanos.Add(time.Since(ensureStart).Nanoseconds())

	joinStart := time.Now()
	for _, participant := range []domaincall.RuntimeParticipant{
		{CallID: callID, AccountID: "acc-a", DeviceID: "dev-a", WithVideo: withVideo},
		{CallID: callID, AccountID: "acc-b", DeviceID: "dev-b", WithVideo: withVideo},
	} {
		if _, err := harness.cluster.JoinSession(ctx, session.SessionID, participant); err != nil {
			return err
		}
	}
	samples.joinNanos.Add(time.Since(joinStart).Nanoseconds())

	currentSessionID := session.SessionID
	if mode == "cluster-migrate" {
		migrateStart := time.Now()
		migrated, err := harness.cluster.MigrateSession(ctx, domaincall.Call{
			ID:              callID,
			ConversationID:  "cluster-load-conv-" + callID,
			ActiveSessionID: currentSessionID,
		})
		if err != nil {
			return err
		}
		currentSessionID = migrated.SessionID
		samples.migrateNanos.Add(time.Since(migrateStart).Nanoseconds())
	}

	statsStart := time.Now()
	if _, err := harness.cluster.SessionStats(ctx, currentSessionID); err != nil {
		return err
	}
	samples.statsNanos.Add(time.Since(statsStart).Nanoseconds())

	closeStart := time.Now()
	if err := harness.cluster.CloseSession(ctx, currentSessionID); err != nil {
		return err
	}
	if currentSessionID != session.SessionID {
		_ = harness.cluster.CloseSession(ctx, session.SessionID)
	}
	samples.closeNanos.Add(time.Since(closeStart).Nanoseconds())

	return nil
}

func negotiateOnce(
	ctx context.Context,
	manager *rtc.Manager,
	sessionID string,
	callID string,
	withVideo bool,
	relayOnly bool,
	turnURL string,
	turnSecret string,
) error {
	config := webrtc.Configuration{}
	if relayOnly {
		username, credential := turnCredential(turnSecret, "load-"+callID, time.Now().UTC().Add(15*time.Minute))
		config.ICETransportPolicy = webrtc.ICETransportPolicyRelay
		config.ICEServers = []webrtc.ICEServer{{
			URLs:       []string{trim(turnURL)},
			Username:   username,
			Credential: credential,
		}}
	}

	client, err := webrtc.NewPeerConnection(config)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if _, err := client.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		return err
	}
	if withVideo {
		if _, err := client.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo); err != nil {
			return err
		}
	}

	offer, err := client.CreateOffer(nil)
	if err != nil {
		return err
	}
	gatherComplete := webrtc.GatheringCompletePromise(client)
	if err := client.SetLocalDescription(offer); err != nil {
		return err
	}
	if relayOnly {
		select {
		case <-gatherComplete:
		case <-time.After(10 * time.Second):
			return fmt.Errorf("timed out waiting for relay-only ICE gathering for %s", callID)
		}
		if !strings.Contains(client.LocalDescription().SDP, " typ relay ") {
			return fmt.Errorf("missing relay candidate for %s", callID)
		}
	}
	signals, err := manager.PublishDescription(ctx, sessionID, domaincall.RuntimeParticipant{
		CallID:    callID,
		AccountID: "acc-a",
		DeviceID:  "dev-a",
		WithVideo: withVideo,
	}, domaincall.SessionDescription{
		Type: "offer",
		SDP:  client.LocalDescription().SDP,
	})
	if err != nil {
		return err
	}
	for _, signal := range signals {
		if signal.Description == nil || signal.Description.Type != "answer" {
			continue
		}
		return client.SetRemoteDescription(webrtc.SessionDescription{
			Type: webrtc.SDPTypeAnswer,
			SDP:  signal.Description.SDP,
		})
	}

	return fmt.Errorf("missing answer for %s", callID)
}

func averageDuration(totalNanos int64, count int) time.Duration {
	if count <= 0 || totalNanos <= 0 {
		return 0
	}
	return time.Duration(totalNanos / int64(count))
}

func turnCredential(secret string, accountID string, expiresAt time.Time) (string, string) {
	username := fmt.Sprintf("%d:%s", expiresAt.UTC().Unix(), trim(accountID))
	mac := hmac.New(sha1.New, []byte(trim(secret)))
	_, _ = mac.Write([]byte(username))
	return username, base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func trim(value string) string {
	return strings.TrimSpace(value)
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}

type clusterHarness struct {
	cluster       *rtc.Cluster
	localManager  *rtc.Manager
	remoteManager *rtc.Manager
	server        *grpc.Server
	listener      net.Listener
}

func newClusterHarness() (*clusterHarness, error) {
	localManager := rtc.NewManager("webrtc://node-a/calls", 15*time.Minute, rtc.WithCandidateHost("127.0.0.1"), rtc.WithUDPPortRange(49100, 49599))
	remoteManager := rtc.NewManager("webrtc://node-b/calls", 15*time.Minute, rtc.WithCandidateHost("127.0.0.1"), rtc.WithUDPPortRange(49600, 49999))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	server := grpc.NewServer()
	callruntimev1.RegisterCallRuntimeServiceServer(server, rtc.NewGRPCRuntimeServer(remoteManager))
	go func() {
		_ = server.Serve(listener)
	}()

	cluster, err := rtc.NewCluster(domaincall.RTCConfig{
		PublicEndpoint: "webrtc://gateway/calls",
		CredentialTTL:  15 * time.Minute,
		NodeID:         "node-a",
		CandidateHost:  "127.0.0.1",
		UDPPortMin:     49100,
		UDPPortMax:     49999,
		Nodes: []domaincall.RTCNode{
			{ID: "node-a", Endpoint: "webrtc://node-a/calls"},
			{ID: "node-b", Endpoint: "webrtc://node-b/calls", ControlEndpoint: listener.Addr().String()},
		},
	}, localManager)
	if err != nil {
		server.Stop()
		_ = listener.Close()
		return nil, err
	}

	return &clusterHarness{
		cluster:       cluster,
		localManager:  localManager,
		remoteManager: remoteManager,
		server:        server,
		listener:      listener,
	}, nil
}

func (h *clusterHarness) Close(ctx context.Context) error {
	if h == nil {
		return nil
	}
	if h.cluster != nil {
		if err := h.cluster.Close(ctx); err != nil {
			return err
		}
	}
	if h.server != nil {
		h.server.Stop()
	}
	if h.listener != nil {
		if err := h.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return err
		}
	}
	return nil
}
