package cli

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/OpceanAI/Doki/pkg/common"
	"github.com/OpceanAI/Doki/pkg/netlink"
)

// dataRootForCLI returns the directory where mesh state lives.
// Honours $DOKI_ROOT for tests, then falls back to the platform data
// directory.
func dataRootForCLI() string {
	if r := os.Getenv("DOKI_ROOT"); r != "" {
		return r
	}
	return common.AppDataDir()
}

// MeshLs lists the peers known to this Doki instance.
func (c *DokiCLI) MeshLs() error {
	sp, err := newStaticPeersCLI()
	if err != nil {
		return err
	}
	peers := sp.List()
	if len(peers) == 0 {
		fmt.Fprintln(os.Stderr, "no peers configured. use `doki link add <id> <addr> --pub <b64>` to add one.")
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 8, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "PEER ID\tNAME\tADDRESS\tLAST SEEN")
	for _, p := range peers {
		last := "-"
		if !p.LastSeen.IsZero() {
			last = formatDuration(timeSince(p.LastSeen))
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.ID, p.Name, p.Addr, last)
	}
	_ = w.Flush()
	return nil
}

// MeshStatus returns the local mesh install id and CA fingerprint.
func (c *DokiCLI) MeshStatus() error {
	root := filepath.Join(dataRootForCLI(), "keys")
	id, err := netlink.NewIdentity(root)
	if err != nil {
		return fmt.Errorf("mesh status: identity: %w", err)
	}
	fmt.Println("install id:    ", id.ShortID())
	fmt.Println("public key:    ", base64.StdEncoding.EncodeToString(id.PublicKey()))
	if ca := id.CACert(); ca != nil {
		fmt.Println("ca fingerprint:", shortFP(ca.Raw))
		fmt.Println("ca expires:    ", ca.NotAfter.Format("2006-01-02"))
	}
	return nil
}

// LinkAdd adds a static peer to the mesh.
func (c *DokiCLI) LinkAdd(id, addr, pubB64 string) error {
	if id == "" || addr == "" || pubB64 == "" {
		return fmt.Errorf("link add: usage: doki link add <peer-id> <host:port> --pub <base64-pubkey>")
	}
	if _, err := base64.StdEncoding.DecodeString(pubB64); err != nil {
		return fmt.Errorf("link add: pub key is not valid base64: %w", err)
	}
	if !looksLikeHostPort(addr) {
		return fmt.Errorf("link add: %q is not a valid host:port", addr)
	}
	sp, err := newStaticPeersCLI()
	if err != nil {
		return err
	}
	existing := sp.Get(id)
	p := &netlink.Peer{
		ID:     id,
		Name:   id,
		Addr:   addr,
		PubKey: pubB64,
	}
	if err := sp.Add(p); err != nil {
		return err
	}
	if existing != nil {
		fmt.Printf("peer %q updated: %s -> %s\n", id, existing.Addr, addr)
	} else {
		fmt.Printf("peer %q added at %s\n", id, addr)
	}
	return nil
}

// LinkRemove removes a peer from the static list.
func (c *DokiCLI) LinkRemove(id string) error {
	if id == "" {
		return fmt.Errorf("link rm: usage: doki link rm <peer-id>")
	}
	sp, err := newStaticPeersCLI()
	if err != nil {
		return err
	}
	if err := sp.Remove(id); err != nil {
		return err
	}
	fmt.Printf("peer %q removed\n", id)
	return nil
}

// LinkShow prints the raw peers.json for debugging.
func (c *DokiCLI) LinkShow() error {
	path := peersPathCLI()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "no peers configured")
			return nil
		}
		return err
	}
	var out interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func newStaticPeersCLI() (*netlink.StaticPeers, error) {
	return netlink.NewStaticPeers(peersPathCLI())
}

func peersPathCLI() string {
	return filepath.Join(dataRootForCLI(), "mesh", "peers.json")
}

func shortFP(b []byte) string {
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%x", sum[:6])
}

func timeSince(t time.Time) time.Duration {
	return time.Since(t)
}

// looksLikeHostPort returns true if s is of the form host:port with a
// numeric port in 1..65535. It does not resolve the host.
func looksLikeHostPort(s string) bool {
	host, port, err := net.SplitHostPort(s)
	if err != nil || host == "" || port == "" {
		return false
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return false
	}
	return true
}
