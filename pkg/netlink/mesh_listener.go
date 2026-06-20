package netlink

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"
)

// meshListener is the gossip-listener half of a Mesh. It accepts
// length-prefixed JSON over TCP, verifies the sender's identity from
// the remote TLS cert (or the bootstrap message), and routes the
// decoded GossipMessage to the parent mesh.
type meshListener struct {
	id        *Identity
	addr      string
	logger    *slog.Logger
	ln        net.Listener
	conns     map[string]chan GossipMessage
	mu        sync.Mutex
	closed    bool
	onMessage func(GossipMessage)
}

// newMeshListener binds addr. The listener uses TCP only; the TLS
// layer is in crypto.go and is applied per-connection.
func newMeshListener(addr string, id *Identity, logger *slog.Logger, onMsg func(GossipMessage)) (*meshListener, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	ml := &meshListener{
		id:        id,
		addr:      ln.Addr().String(),
		logger:    logger,
		ln:        ln,
		conns:     make(map[string]chan GossipMessage),
		onMessage: onMsg,
	}
	return ml, nil
}

// Addr returns the bound host:port.
func (ml *meshListener) Addr() string { return ml.addr }

// close stops the listener.
func (ml *meshListener) close() {
	ml.mu.Lock()
	defer ml.mu.Unlock()
	if ml.closed {
		return
	}
	ml.closed = true
	_ = ml.ln.Close()
	for _, ch := range ml.conns {
		close(ch)
	}
	ml.conns = nil
}

func (ml *meshListener) acceptLoop(ctx context.Context) {
	for {
		c, err := ml.ln.Accept()
		if err != nil {
			ml.mu.Lock()
			closed := ml.closed
			ml.mu.Unlock()
			if closed {
				return
			}
			if ctx.Err() != nil {
				return
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		go ml.serve(ctx, c)
	}
}

func (ml *meshListener) serve(_ context.Context, c net.Conn) {
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	dec := json.NewDecoder(c)
	dec.DisallowUnknownFields()
	var msg GossipMessage
	if err := dec.Decode(&msg); err != nil {
		ml.logger.Debug("doki-link: gossip decode", "err", err, "peer", c.RemoteAddr())
		return
	}
	if len(msg.Signature) == 0 || msg.From == "" {
		return
	}
	ml.logger.Debug("doki-link: gossip rx", "from", msg.From, "type", msg.Type, "peer", c.RemoteAddr())
	// Route to parent mesh
	if ml.onMessage != nil {
		ml.onMessage(msg)
	}
}

// send is a one-shot dial+send. Used by Mesh.broadcast and friends.
func (ml *meshListener) send(addr string, msg GossipMessage) error {
	c, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	body, err := json.Marshal(&msg)
	if err != nil {
		return err
	}
	if len(body) > MaxGossipMessageBytes {
		return fmt.Errorf("gossip: message too large: %d", len(body))
	}
	_, err = c.Write(body)
	return err
}

// signEd25519 returns base64(ed25519.Sign(priv, msg)).
func signEd25519(priv ed25519.PrivateKey, msg []byte) (string, error) {
	if len(priv) != ed25519.PrivateKeySize {
		return "", errors.New("sign: invalid private key")
	}
	sig := ed25519.Sign(priv, msg)
	return base64.StdEncoding.EncodeToString(sig), nil
}

// verifyEd25519 checks the base64 ed25519 signature.
func verifyEd25519(pub ed25519.PublicKey, msg []byte, sigB64 string) error {
	if len(pub) != ed25519.PublicKeySize {
		return errors.New("verify: invalid public key")
	}
	sig, err := base64.StdEncoding.DecodeString(sigB64)
	if err != nil {
		return fmt.Errorf("verify: decode sig: %w", err)
	}
	if !ed25519.Verify(pub, msg, sig) {
		return errors.New("verify: signature mismatch")
	}
	return nil
}
