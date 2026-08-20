package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/TBG-Chance/SentinelBox/internal/buildinfo"
	"github.com/TBG-Chance/SentinelBox/internal/config"
)

func TestServeHealthAndGracefulShutdown(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Server.Listen = listener.Addr().String()
	cfg.Server.ShutdownTimeout = config.NewDuration(2 * time.Second)
	cfg.Database.Path = filepath.Join(t.TempDir(), "sentinel.db")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application, err := New(context.Background(), cfg, logger, buildinfo.Info{Version: "test"})
	if err != nil {
		listener.Close()
		t.Fatal(err)
	}
	defer application.Close()

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- application.serve(ctx, listener) }()

	client := &http.Client{Timeout: time.Second}
	var response *http.Response
	for attempt := 0; attempt < 20; attempt++ {
		response, err = client.Get("http://" + listener.Addr().String() + "/api/v1/health")
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		cancel()
		t.Fatalf("health request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("health status = %d", response.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		cancel()
		t.Fatal(err)
	}
	if payload["status"] != "healthy" {
		cancel()
		t.Fatalf("health payload = %#v", payload)
	}

	cancel()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatalf("serve returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("application did not shut down")
	}
}
