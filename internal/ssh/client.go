// Package ssh provides resilient SSH client connection and command execution
// for remote router management.
package ssh

import (
	"fmt"
	"net"
	"strconv"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// Connect establishes an SSH connection to the router with timeout handling.
func Connect(host string, port int, user, password string) (*gossh.Client, error) {
	sshConfig := &gossh.ClientConfig{
		User:            user,
		Auth:            []gossh.AuthMethod{gossh.Password(password)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))

	dialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 5 * time.Second,
	}

	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	// Set handshake timeout to prevent hanging on banner exchange.
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	sshConn, chans, reqs, err := gossh.NewClientConn(conn, addr, sshConfig)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// Reset deadline for persistent normal use.
	_ = conn.SetDeadline(time.Time{})

	return gossh.NewClient(sshConn, chans, reqs), nil
}

// ConnectWithConfig is a convenience wrapper that accepts a ConnectionInfo struct.
func ConnectWithConfig(info ConnectionInfo) (*gossh.Client, error) {
	return Connect(info.Host, info.Port, info.User, info.Password)
}

// ExecFunc is the signature for a function that executes a remote SSH command.
type ExecFunc func(client *gossh.Client, cmd string) (string, error)

// ConnectionInfo holds all parameters needed to establish an SSH connection.
type ConnectionInfo struct {
	Host     string
	Port     int
	User     string
	Password string
}

// EnsureClient validates an existing client or creates a new one using the provided reconnect function.
// It tests the connection by creating a session; if that fails, it calls reconnect.
func EnsureClient(client *gossh.Client, reconnect func() (*gossh.Client, error)) (*gossh.Client, error) {
	if client == nil {
		return reconnect()
	}

	// Test the connection with a timeout to prevent hanging on half-dead TCP connections.
	type sessResult struct {
		sess *gossh.Session
		err  error
	}
	sessChan := make(chan sessResult, 1)
	go func() {
		sess, err := client.NewSession()
		sessChan <- sessResult{sess, err}
	}()

	var sessErr error
	select {
	case res := <-sessChan:
		if res.sess != nil {
			res.sess.Close()
		}
		sessErr = res.err
	case <-time.After(3 * time.Second):
		sessErr = fmt.Errorf("NewSession timeout (connection is dead)")
	}

	if sessErr == nil {
		return client, nil
	}

	// Close the dead client to release socket resources.
	_ = client.Close()

	// Connection is dead — try to reconnect.
	return reconnect()
}

// Exec runs a command on the remote host via the given SSH client.
// It uses EnsureClient internally for resilience against dropped connections.
func Exec(client *gossh.Client, cmd string, reconnect func() (*gossh.Client, error)) (string, error) {
	activeClient, err := EnsureClient(client, reconnect)
	if err != nil {
		return "", err
	}

	type result struct {
		out string
		err error
	}
	ch := make(chan result, 1)

	go func() {
		sess, err := activeClient.NewSession()
		if err != nil {
			ch <- result{"", err}
			return
		}
		defer sess.Close()

		out, err := sess.CombinedOutput(cmd)
		ch <- result{string(out), err}
	}()

	select {
	case res := <-ch:
		return res.out, res.err
	case <-time.After(20 * time.Second):
		return "", fmt.Errorf("ssh command timeout after 20 seconds")
	}
}
