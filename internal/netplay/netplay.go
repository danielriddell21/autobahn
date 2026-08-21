// Package netplay carries a two-player chase over the network.
//
// One machine hosts: it owns the simulation, and its word is final. The other
// joins, sends the controls of the police unit it is driving, and draws what
// the host tells it. There is no prediction and no rollback — this is a chase
// between two cars, not a fighting game, and a fifty millisecond lag on a
// police car behind you is not worth the complexity.
//
// Only moving things cross the wire. The city is generated from a seed, so both
// ends build exactly the same streets, buildings and signals from the same
// number and never speak of them again. What is left is a few dozen poses,
// which is small enough to send whole at twenty times a second rather than
// bothering with deltas.
package netplay

import (
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

// Port is the default port the host listens on.
const Port = "7777"

// SnapshotRate is how many world updates the host sends per second.
const SnapshotRate = 20

// Pose is one vehicle as the wire sees it: where it is, which way it points,
// and the few flags the renderer needs.
type Pose struct {
	X, Z  float32
	Yaw   float32
	Speed float32
	// Flags carries the states a remote viewer cannot infer from a pose.
	Flags uint8
}

// Pose flags.
const (
	// FlagPursuing marks a police unit running to a call, so its lights flash.
	FlagPursuing uint8 = 1 << iota
	// FlagBraking lights the brake lamps.
	FlagBraking
)

// Snapshot is the whole moving world at one instant.
type Snapshot struct {
	// Clock is the host's simulation time, which drives the signals. Sending it
	// keeps every traffic light on both machines showing the same aspect.
	Clock float32
	// Runner is the car being chased.
	Runner Pose
	// Agents are the traffic and police, in the order the seeded world creates
	// them, so the client can match them to its own.
	Agents []Pose
	// Chaser is the index in Agents of the unit the joining player drives.
	Chaser int

	WantedLevel int
	WantedState int
	Elapsed     float32
	Over        bool
	Winner      string
}

// Input is one frame of a joining player's controls.
type Input struct {
	Throttle  float32
	Brake     float32
	Steer     float32
	Handbrake bool
	Reverse   bool
}

// Host accepts a single joining player and exchanges state with them.
type Host struct {
	listener net.Listener

	mu      sync.Mutex
	input   Input
	joined  bool
	lastErr error
	enc     *gob.Encoder
	conn    net.Conn
}

// Listen starts a host on the given address, which may be just ":7777".
func Listen(addr string) (*Host, error) {
	var cfg net.ListenConfig
	l, err := cfg.Listen(context.Background(), "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listening on %s: %w", addr, err)
	}
	h := &Host{listener: l}
	go h.accept()
	return h, nil
}

// Addr returns the address the host is listening on.
func (h *Host) Addr() string { return h.listener.Addr().String() }

// Joined reports whether a player has connected.
func (h *Host) Joined() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.joined
}

// Input returns the joining player's most recent controls. A player who has
// not connected, or who has gone quiet, simply sends nothing, and the unit they
// were driving is left to the simulation.
func (h *Host) Input() (Input, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.input, h.joined
}

// Send publishes the current world to the joining player.
func (h *Host) Send(s Snapshot) {
	h.mu.Lock()
	enc, joined := h.enc, h.joined
	h.mu.Unlock()
	if !joined || enc == nil {
		return
	}
	if err := enc.Encode(s); err != nil {
		h.drop(err)
	}
}

// Close stops listening and disconnects any player.
func (h *Host) Close() error {
	h.mu.Lock()
	conn := h.conn
	h.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
	return h.listener.Close()
}

func (h *Host) accept() {
	for {
		conn, err := h.listener.Accept()
		if err != nil {
			return // the listener was closed
		}
		h.mu.Lock()
		// One player at a time: a second caller replaces the first.
		if h.conn != nil {
			_ = h.conn.Close()
		}
		h.conn, h.enc, h.joined = conn, gob.NewEncoder(conn), true
		h.mu.Unlock()
		go h.read(conn)
	}
}

func (h *Host) read(conn net.Conn) {
	dec := gob.NewDecoder(conn)
	for {
		var in Input
		if err := dec.Decode(&in); err != nil {
			h.drop(err)
			return
		}
		h.mu.Lock()
		h.input = in
		h.mu.Unlock()
	}
}

func (h *Host) drop(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conn != nil {
		_ = h.conn.Close()
		h.conn = nil
	}
	h.enc, h.joined, h.input = nil, false, Input{}
	if !errors.Is(err, net.ErrClosed) {
		h.lastErr = err
	}
}

// Err returns the last connection error, if any.
func (h *Host) Err() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastErr
}

// Client is the joining side: it sends controls and receives the world.
type Client struct {
	conn net.Conn
	enc  *gob.Encoder

	mu     sync.Mutex
	latest Snapshot
	got    bool
	err    error
}

// Join connects to a host. The address may omit the port, in which case the
// default is used.
func Join(addr string) (*Client, error) {
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, Port)
	}
	d := net.Dialer{Timeout: 8 * time.Second}
	conn, err := d.DialContext(context.Background(), "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("joining %s: %w", addr, err)
	}
	c := &Client{conn: conn, enc: gob.NewEncoder(conn)}
	go c.read()
	return c, nil
}

// Send publishes this player's controls to the host.
func (c *Client) Send(in Input) {
	if err := c.enc.Encode(in); err != nil {
		c.fail(err)
	}
}

// Snapshot returns the most recent world the host sent, and whether one has
// arrived yet.
func (c *Client) Snapshot() (Snapshot, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.latest, c.got
}

// Err returns the error that ended the connection, if it has ended.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Close disconnects from the host.
func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) read() {
	dec := gob.NewDecoder(c.conn)
	for {
		var s Snapshot
		if err := dec.Decode(&s); err != nil {
			c.fail(err)
			return
		}
		c.mu.Lock()
		c.latest, c.got = s, true
		c.mu.Unlock()
	}
}

func (c *Client) fail(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err == nil && !errors.Is(err, net.ErrClosed) {
		c.err = err
	}
}
