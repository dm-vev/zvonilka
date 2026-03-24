//go:build !race

package s3

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

	accessKey, secretKey := bootstrapS4Credentials(t, db.endpoint)

	cfg := config.Configuration{
		Infrastructure: config.InfrastructureConfig{
			ObjectStore: config.ObjectStorageConfig{
				Enabled:         true,
				Endpoint:        db.endpoint,
				Region:          "us-east-1",
				Bucket:          "zvonilka-media",
				AccessKeyID:     accessKey,
				SecretAccessKey: secretKey,
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
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl is not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	certDir := t.TempDir()
	certPath := filepath.Join(certDir, "cert.pem")
	keyPath := filepath.Join(certDir, "key.pem")
	certCmd := exec.CommandContext(
		ctx,
		"openssl",
		"req",
		"-x509",
		"-newkey",
		"rsa:4096",
		"-keyout",
		keyPath,
		"-out",
		certPath,
		"-days",
		"365",
		"-nodes",
		"-subj",
		"/CN=localhost",
	)
	if output, err := certCmd.CombinedOutput(); err != nil {
		t.Skipf("generate s4 tls certs: %v: %s", err, strings.TrimSpace(string(output)))
	}

	cmd := exec.CommandContext(
		ctx,
		"docker",
		"run",
		"-d",
		"--rm",
		"-e",
		"S4_ROOT_PASSWORD=password12345",
		"-e",
		"S4_REGION=us-east-1",
		"-e",
		"S4_TLS_CERT=/certs/cert.pem",
		"-e",
		"S4_TLS_KEY=/certs/key.pem",
		"-v",
		certDir+":/certs:ro",
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

	endpoint := fmt.Sprintf("https://localhost:%s", hostPort)
	return &dockerS4{containerID: containerID, endpoint: endpoint}
}

type adminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type adminLoginResponse struct {
	Token string `json:"token"`
}

type adminUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type adminUserResponse struct {
	ID string `json:"id"`
}

type adminCredentialsResponse struct {
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
}

func bootstrapS4Credentials(t *testing.T, endpoint string) (string, string) {
	t.Helper()

	accessKey := strings.TrimSpace(os.Getenv("S4_ACCESS_KEY_ID"))
	secretKey := strings.TrimSpace(os.Getenv("S4_SECRET_ACCESS_KEY"))
	if accessKey != "" || secretKey != "" {
		if accessKey == "" || secretKey == "" {
			t.Fatal("set both S4_ACCESS_KEY_ID and S4_SECRET_ACCESS_KEY, or neither")
		}
		return accessKey, secretKey
	}

	client := insecureHTTPClient()
	rootPassword := strings.TrimSpace(os.Getenv("S4_ROOT_PASSWORD"))
	if rootPassword == "" {
		rootPassword = "password12345"
	}

	deadline := time.Now().Add(15 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		accessKey, secretKey, lastErr = tryBootstrapS4Credentials(client, endpoint, rootPassword, t.Name())
		if lastErr == nil {
			return accessKey, secretKey
		}
		time.Sleep(1 * time.Second)
	}

	t.Skipf("bootstrap S4 credentials for %s: %v", endpoint, lastErr)
	return "", ""
}

func tryBootstrapS4Credentials(client *http.Client, endpoint, rootPassword, testName string) (string, string, error) {
	token, err := loginS4Admin(client, endpoint, "root", rootPassword)
	if err != nil {
		return "", "", err
	}

	username := fmt.Sprintf("zvonilka-s4-%s", sanitizeTestName(testName))
	password := "password123"
	userID, err := createS4User(client, endpoint, token, username, password)
	if err != nil {
		return "", "", err
	}

	credentials, err := createS4Credentials(client, endpoint, token, userID)
	if err != nil {
		return "", "", err
	}
	if credentials.AccessKey == "" || credentials.SecretKey == "" {
		return "", "", fmt.Errorf("expected generated s3 credentials, got %+v", credentials)
	}

	return credentials.AccessKey, credentials.SecretKey, nil
}

func insecureHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func loginS4Admin(client *http.Client, endpoint, username, password string) (string, error) {
	var response adminLoginResponse
	body, err := postJSONResponse(client, endpoint+"/api/admin/login", adminLoginRequest{
		Username: username,
		Password: password,
	}, nil)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode admin login response: %w", err)
	}
	if response.Token == "" {
		return "", fmt.Errorf("expected admin login token")
	}

	return response.Token, nil
}

func createS4User(client *http.Client, endpoint, token, username, password string) (string, error) {
	var response adminUserResponse
	body, err := postJSONResponse(client, endpoint+"/api/admin/users", adminUserRequest{
		Username: username,
		Password: password,
		Role:     "Writer",
	}, map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode create user response: %w", err)
	}
	if response.ID == "" {
		return "", fmt.Errorf("expected user id")
	}

	return response.ID, nil
}

func createS4Credentials(client *http.Client, endpoint, token, userID string) (adminCredentialsResponse, error) {
	var response adminCredentialsResponse
	body, err := postJSONResponse(client, endpoint+"/api/admin/users/"+userID+"/credentials", nil, map[string]string{"Authorization": "Bearer " + token})
	if err != nil {
		return adminCredentialsResponse{}, err
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return adminCredentialsResponse{}, fmt.Errorf("decode create credentials response: %w", err)
	}

	return response, nil
}

func postJSONResponse(client *http.Client, rawURL string, payload any, headers map[string]string) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequest(http.MethodPost, rawURL, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("post %s failed: status=%d body=%s", rawURL, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return respBody, nil
}

func postJSON(t *testing.T, client *http.Client, rawURL string, payload any, headers map[string]string, into any) {
	t.Helper()

	respBody, err := postJSONResponse(client, rawURL, payload, headers)
	if err != nil {
		t.Fatal(err)
	}

	if into == nil {
		return
	}

	if err := json.Unmarshal(respBody, into); err != nil {
		t.Fatalf("decode response: %v body=%s", err, strings.TrimSpace(string(respBody)))
	}
}

func sanitizeTestName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, " ", "-")
	return name
}
