package pubengine

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func lifecycleConfig(t *testing.T) SiteConfig {
	t.Helper()
	dir := t.TempDir()
	return SiteConfig{AdminPassword: "test-password", SessionSecret: strings.Repeat("s", 32), Addr: "127.0.0.1:0", DatabasePath: filepath.Join(dir, "blog.db"), AnalyticsEnabled: true, AnalyticsDatabasePath: filepath.Join(dir, "analytics", "analytics.db"), ShutdownTimeout: 3 * time.Second}
}

func TestShutdownDrainsRequestsAndClosesResources(t *testing.T) {
	for i := 0; i < 3; i++ {
		entered, release := make(chan struct{}), make(chan struct{})
		a := New(lifecycleConfig(t), ViewFuncs{}, WithCustomRoutes(func(a *App) {
			a.Echo.GET("/hold/", func(c echo.Context) error {
				close(entered)
				<-release
				if _, err := a.Cache.ListPosts(""); err != nil {
					return err
				}
				return c.String(200, "finished")
			})
		}))
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- a.StartContext(ctx) }()
		var address string
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			a.lifecycleMu.Lock()
			if a.Echo.Listener != nil {
				address = a.Echo.Listener.Addr().String()
			}
			a.lifecycleMu.Unlock()
			if address != "" {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if address == "" {
			cancel()
			t.Fatal("listener did not start")
		}
		response := make(chan error, 1)
		go func() {
			client := http.Client{Timeout: 5 * time.Second}
			res, err := client.Get("http://" + address + "/hold/")
			if err != nil {
				response <- err
				return
			}
			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			if res.StatusCode != 200 || string(body) != "finished" {
				t.Errorf("response=%d %s", res.StatusCode, body)
			}
			response <- err
		}()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			cancel()
			t.Fatal("handler did not start")
		}
		cancel()
		select {
		case err := <-done:
			t.Fatalf("shutdown returned before request finished: %v", err)
		case <-time.After(20 * time.Millisecond):
		}
		close(release)
		if err := <-response; err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(4 * time.Second):
			t.Fatal("shutdown timed out")
		}
		if err := a.Close(); err != nil {
			t.Fatal(err)
		}
		if err := a.Store.db.Ping(); err == nil {
			t.Fatal("database is still open")
		}
		select {
		case <-a.loginLimiter.stopped:
		default:
			t.Fatal("login worker is still running")
		}
		if err := a.Start(); err == nil {
			t.Fatal("closed app restarted")
		}
	}
}

func TestFailedInitializationClosesEarlierResources(t *testing.T) {
	cfg := lifecycleConfig(t)
	block := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(block, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.AnalyticsDatabasePath = filepath.Join(block, "analytics.db")
	a := New(cfg, ViewFuncs{})
	if err := a.Start(); err == nil {
		t.Fatal("expected initialization failure")
	}
	if a.Store == nil {
		t.Fatal("blog store was never opened")
	}
	if err := a.Store.db.Ping(); err == nil {
		t.Fatal("blog store leaked")
	}
	select {
	case <-a.loginLimiter.stopped:
	default:
		t.Fatal("limiter leaked")
	}
}
