// Package katago runs KataGo's JSON analysis engine as a long-lived
// subprocess. One process serves every feature that needs a neural net:
// next-move suggestions, life and death, contested territory and strength all
// come from the same winrate / score / ownership output.
package katago

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Config locates KataGo. All three paths live outside the repo and are
// user-configurable.
type Config struct {
	Binary string `json:"binary"`
	Model  string `json:"model"`
	Config string `json:"config"`
}

// Valid reports whether the three files exist.
func (c Config) Valid() error {
	for name, p := range map[string]string{"binary": c.Binary, "model": c.Model, "config": c.Config} {
		if p == "" {
			return fmt.Errorf("katago %s path is not set", name)
		}
		if _, err := os.Stat(p); err != nil {
			return fmt.Errorf("katago %s: %w", name, err)
		}
	}
	return nil
}

// ErrUnavailable is returned when KataGo is not running. Features that need
// it should disable themselves rather than fail loudly.
var ErrUnavailable = errors.New("katago is not available")

type Client struct {
	cfg     Config
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	wmu     sync.Mutex
	mu      sync.Mutex
	wait    map[string]chan *Response
	closed  bool
	done    chan struct{}
	nextID  atomic.Uint64
	stderr  *ring
	Version string
}

// Start launches KataGo and waits until it answers a version query.
func Start(ctx context.Context, cfg Config) (*Client, error) {
	if err := cfg.Valid(); err != nil {
		return nil, err
	}
	cmd := exec.Command(cfg.Binary, "analysis", "-config", cfg.Config, "-model", cfg.Model)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	c := &Client{cfg: cfg, cmd: cmd, stdin: stdin, wait: map[string]chan *Response{}, done: make(chan struct{}), stderr: newRing(40)}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start katago: %w", err)
	}
	go func() {
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			c.stderr.add(sc.Text())
		}
	}()
	go c.readLoop(stdout)

	rctx, cancel := context.WithTimeout(ctx, 3*time.Minute) // first run tunes the GPU
	defer cancel()
	resp, err := c.request(rctx, map[string]any{"action": "query_version"})
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("katago did not become ready: %w\n%s", err, c.stderr.String())
	}
	c.Version = resp.Version
	return c, nil
}

func (c *Client) readLoop(r io.Reader) {
	defer close(c.done)
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var resp Response
		if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
			continue
		}
		c.mu.Lock()
		ch := c.wait[resp.ID]
		c.mu.Unlock()
		if ch == nil {
			continue
		}
		select {
		case ch <- &resp:
		default:
			// A slow reader drops intermediate (during-search) updates but
			// must never block the read loop.
			if !resp.IsDuringSearch {
				go func(ch chan *Response, r Response) { ch <- &r }(ch, resp)
			}
		}
	}
	c.mu.Lock()
	c.closed = true
	for id, ch := range c.wait {
		select {
		case ch <- &Response{ID: id, Error: "katago exited"}:
		default:
		}
	}
	c.mu.Unlock()
}

// request sends one JSON line and waits for its final response.
func (c *Client) request(ctx context.Context, q map[string]any) (*Response, error) {
	id, _ := q["id"].(string)
	if id == "" {
		id = fmt.Sprintf("q%d", c.nextID.Add(1))
		q["id"] = id
	}
	ch := make(chan *Response, 8)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrUnavailable
	}
	c.wait[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.wait, id)
		c.mu.Unlock()
	}()

	line, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	c.wmu.Lock()
	_, err = c.stdin.Write(append(line, '\n'))
	c.wmu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	for {
		select {
		case resp := <-ch:
			if resp.Error != "" {
				return resp, errors.New(resp.Error)
			}
			if resp.IsDuringSearch {
				continue
			}
			return resp, nil
		case <-ctx.Done():
			// Tell KataGo to stop wasting GPU time on an abandoned query.
			c.fire(map[string]any{"id": id + "-t", "action": "terminate", "terminateId": id})
			return nil, ctx.Err()
		case <-c.done:
			return nil, ErrUnavailable
		}
	}
}

func (c *Client) fire(q map[string]any) {
	line, _ := json.Marshal(q)
	c.wmu.Lock()
	c.stdin.Write(append(line, '\n'))
	c.wmu.Unlock()
}

// Analyze runs one analysis query.
func (c *Client) Analyze(ctx context.Context, q Query) (*Response, error) {
	return c.request(ctx, q.toMap())
}

// Running reports whether the subprocess is still alive.
func (c *Client) Running() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

// Close stops KataGo.
func (c *Client) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.stdin.Close()
	select {
	case <-c.done:
	case <-time.After(3 * time.Second):
		c.cmd.Process.Kill()
	}
	return c.cmd.Wait()
}

// Log returns the tail of KataGo's stderr, for diagnostics.
func (c *Client) Log() string { return c.stderr.String() }

type ring struct {
	mu    sync.Mutex
	lines []string
	n     int
}

func newRing(n int) *ring { return &ring{n: n} }
func (r *ring) add(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, s)
	if len(r.lines) > r.n {
		r.lines = r.lines[1:]
	}
}
func (r *ring) String() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.lines, "\n")
}
