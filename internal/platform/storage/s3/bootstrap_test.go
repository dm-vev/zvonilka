//go:build !race

package s3

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/dm-vev/zvonilka/internal/domain/storage"
	"github.com/dm-vev/zvonilka/internal/platform/config"
	"github.com/dm-vev/zvonilka/internal/testsupport/dockermutex"
)

func TestBootstrapOpenUsesS4CompatibleBackend(t *testing.T) {
	if os.Getenv("RUN_S4_INTEGRATION") != "1" {
		t.Skip("set RUN_S4_INTEGRATION=1 to run the S4 integration test")
	}

	db := openDockerS4(t)
	t.Cleanup(func() {
		_ = db.Close()
	})

	cfg := config.Configuration{
		Infrastructure: config.InfrastructureConfig{
			ObjectStore: config.ObjectStorageConfig{
				Enabled:         true,
				Endpoint:        db.endpoint,
				Region:          "us-east-1",
				Bucket:          "zvonilka-media",
				AccessKeyID:     "test-access",
				SecretAccessKey: "test-secret",
				ForcePathStyle:  true,
				UseSSL:          false,
			},
		},
	}

	bootstrap := NewBootstrap(cfg)
	var provider *Provider
	var err error
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		provider, err = bootstrap.Open(context.Background())
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		t.Fatalf("open bootstrap: %v", err)
	}
	if provider == nil {
		t.Fatal("expected provider")
	}
	if provider.Bucket() != "zvonilka-media" {
		t.Fatalf("unexpected bucket: %s", provider.Bucket())
	}

	uploaded, err := provider.PutObject(context.Background(), "media/test/object", bytes.NewReader([]byte("hello")), 5, storage.PutObjectOptions{
		ContentType: "text/plain",
		Metadata:    map[string]string{"sha256": "abc123"},
	})
	if err != nil {
		t.Fatalf("put object: %v", err)
	}
	if uploaded.ContentLength != 5 {
		t.Fatalf("expected content length 5, got %d", uploaded.ContentLength)
	}

	head, err := provider.HeadObject(context.Background(), "media/test/object")
	if err != nil {
		t.Fatalf("head object: %v", err)
	}
	if head.Key != "media/test/object" {
		t.Fatalf("unexpected key: %s", head.Key)
	}
	if head.ContentLength != 5 {
		t.Fatalf("expected content length 5, got %d", head.ContentLength)
	}

	body, object, err := provider.GetObject(context.Background(), "media/test/object")
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	defer body.Close()
	payload, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("read object: %v", err)
	}
	if string(payload) != "hello" {
		t.Fatalf("unexpected body %q", string(payload))
	}
	if object.Key != "media/test/object" {
		t.Fatalf("unexpected object info: %+v", object)
	}

	putTarget, err := provider.PresignPutObject(context.Background(), "media/test/signed", time.Minute, storage.PutObjectOptions{
		ContentType: "text/plain",
	})
	if err != nil {
		t.Fatalf("presign put: %v", err)
	}
	if putTarget.Method != "PUT" || putTarget.URL == "" {
		t.Fatalf("unexpected presigned put: %+v", putTarget)
	}

	getTarget, err := provider.PresignGetObject(context.Background(), "media/test/object", time.Minute)
	if err != nil {
		t.Fatalf("presign get: %v", err)
	}
	if getTarget.Method != "GET" || getTarget.URL == "" {
		t.Fatalf("unexpected presigned get: %+v", getTarget)
	}

	if err := provider.DeleteObject(context.Background(), "media/test/object"); err != nil {
		t.Fatalf("delete object: %v", err)
	}
	if _, err := provider.HeadObject(context.Background(), "media/test/object"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
}

type dockerS4 struct {
	containerID string
	endpoint    string
}

func (d *dockerS4) Close() error {
	if d == nil || d.containerID == "" {
		return nil
	}

	return exec.Command("docker", "rm", "-f", d.containerID).Run()
}

func openDockerS4(t *testing.T) *dockerS4 {
	t.Helper()

	unlock := dockermutex.Acquire(t, "s4-integration")
	t.Cleanup(unlock)

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		"docker",
		"run",
		"-d",
		"--rm",
		"-e",
		"S4_ACCESS_KEY_ID=test-access",
		"-e",
		"S4_SECRET_ACCESS_KEY=test-secret",
		"-e",
		"S4_REGION=us-east-1",
		"-p",
		"127.0.0.1::9000",
		"s4core/s4core:latest",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("docker s4 unavailable: %v: %s", err, strings.TrimSpace(string(output)))
	}

	containerID := strings.TrimSpace(string(output))
	if containerID == "" {
		t.Skip("docker returned an empty container id")
	}

	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", containerID).Run()
	})

	portOut, err := exec.CommandContext(ctx, "docker", "port", containerID, "9000/tcp").CombinedOutput()
	if err != nil {
		t.Skipf("lookup s4 port: %v: %s", err, strings.TrimSpace(string(portOut)))
	}
	hostPort := strings.TrimSpace(string(portOut))
	if hostPort == "" {
		t.Skip("docker did not report a mapped port")
	}
	hostPort = hostPort[strings.LastIndex(hostPort, ":")+1:]

	endpoint := fmt.Sprintf("http://127.0.0.1:%s", hostPort)
	return &dockerS4{containerID: containerID, endpoint: endpoint}
}
