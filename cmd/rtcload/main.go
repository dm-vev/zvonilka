package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/webrtc/v4"

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
	CloseAvg           time.Duration `json:"close_avg"`
	GoMaxProcs         int           `json:"gomaxprocs"`
	GoRoutines         int           `json:"goroutines"`
	EstimatedPeakPeers int           `json:"estimated_peak_peers"`
}

type timings struct {
	ensureNanos    atomic.Int64
	joinNanos      atomic.Int64
	negotiateNanos atomic.Int64
	statsNanos     atomic.Int64
	closeNanos     atomic.Int64
}

func main() {
	var (
		mode        = flag.String("mode", "negotiate", "load mode: lifecycle|negotiate")
		sessions    = flag.Int("sessions", 100, "number of sessions to execute")
		concurrency = flag.Int("concurrency", 8, "number of concurrent workers")
		withVideo   = flag.Bool("with-video", true, "whether participants join with video")
	)
	flag.Parse()

	if *sessions <= 0 {
		fatalf("sessions must be > 0")
	}
	if *concurrency <= 0 {
		fatalf("concurrency must be > 0")
	}
	if *mode != "lifecycle" && *mode != "negotiate" {
		fatalf("unsupported mode %q", *mode)
	}

	manager := rtc.NewManager(
		"webrtc://gateway/calls",
		15*time.Minute,
		rtc.WithCandidateHost("127.0.0.1"),
		rtc.WithUDPPortRange(47000, 49000),
	)
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
				if err := runScenario(ctx, manager, *mode, *withVideo, index, &samples); err != nil {
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
		CloseAvg:           averageDuration(samples.closeNanos.Load(), *sessions),
		GoMaxProcs:         runtime.GOMAXPROCS(0),
		GoRoutines:         runtime.NumGoroutine(),
		EstimatedPeakPeers: *concurrency * 2,
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
	mode string,
	withVideo bool,
	index int,
	samples *timings,
) error {
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
		if err := negotiateOnce(ctx, manager, session.SessionID, callID, withVideo); err != nil {
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

func negotiateOnce(
	ctx context.Context,
	manager *rtc.Manager,
	sessionID string,
	callID string,
	withVideo bool,
) error {
	client, err := webrtc.NewPeerConnection(webrtc.Configuration{})
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
	if err := client.SetLocalDescription(offer); err != nil {
		return err
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

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(2)
}
