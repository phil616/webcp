package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"web-panel/internal/domain"
	"web-panel/internal/openresty"
	"web-panel/internal/store"
)

func TestStreamIntegration(t *testing.T) {
	integrationOnly(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	tcp, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcp.Close()
	go func() {
		for {
			c, e := tcp.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	udp, err := net.ListenPacket("udp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	go func() {
		buf := make([]byte, 65535)
		for {
			n, a, e := udp.ReadFrom(buf)
			if e != nil {
				return
			}
			udp.WriteTo(buf[:n], a)
		}
	}()
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	name := "resty-l4-test-" + domain.NewID()[:10]
	d := &openresty.Docker{Root: s.Root, Image: "1panel/openresty:1.27.1.2-5-1-focal", Container: name, HTTPBind: "127.0.0.1:" + freePort(t), HTTPSBind: "127.0.0.1:" + freePort(t)}
	t.Cleanup(func() {
		out, _ := exec.Command("docker", "ps", "-aq", "--filter", "label=resty.manager.root="+s.Root).Output()
		for _, id := range strings.Fields(string(out)) {
			exec.Command("docker", "rm", "-f", id).Run()
		}
	})
	m, err := New(s, d)
	if err != nil {
		t.Fatal(err)
	}
	hostPort, _ := strconv.Atoi(freePort(t))
	p, err := m.SaveStream(revisionOf(t, s), domain.StreamProxy{Name: "tcp", Protocol: "tcp", ListenAddress: "127.0.0.1", ListenPort: hostPort, TargetHost: "host.docker.internal", TargetPort: tcp.Addr().(*net.TCPAddr).Port, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	q := p
	q.ID = ""
	q.Name = "udp"
	q.Protocol = "udp"
	q.TargetPort = udp.LocalAddr().(*net.UDPAddr).Port
	q.UDPResponses = 1
	q, err = m.SaveStream(revisionOf(t, s), q)
	if err != nil {
		t.Fatal(err)
	}
	apply := func() {
		t.Helper()
		if err := m.Apply(ctx, revisionOf(t, s)); err != nil {
			t.Fatal(err)
		}
	}
	echo := func(protocol string) {
		t.Helper()
		var last error
		deadline := time.Now().Add(10 * time.Second)
		for {
			c, e := net.DialTimeout(protocol, fmt.Sprintf("127.0.0.1:%d", hostPort), time.Second)
			if e == nil {
				c.SetDeadline(time.Now().Add(time.Second))
				payload := []byte("hello\x00\xffTLS passthrough")
				_, e = c.Write(payload)
				if e == nil {
					buf := make([]byte, len(payload))
					_, e = io.ReadFull(c, buf)
					if e == nil && string(buf) != string(payload) {
						e = fmt.Errorf("payload changed: %q", buf)
					}
				}
				c.Close()
			}
			if e == nil {
				return
			}
			last = e
			if time.Now().After(deadline) {
				t.Fatalf("%s forwarding: %v\n%s", protocol, last, dockerCommand(t, "logs", name))
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	apply()
	echo("tcp")
	echo("udp")
	id := dockerCommand(t, "inspect", "--format", "{{.Id}}", name)
	p.ProxyTimeout = 120
	p, err = m.SaveStream(revisionOf(t, s), p)
	if err != nil {
		t.Fatal(err)
	}
	apply()
	if dockerCommand(t, "inspect", "--format", "{{.Id}}", name) != id {
		t.Fatal("timeout update replaced container")
	}
	echo("tcp")
	// Changing the destination and enabling outbound PROXY protocol must reload
	// the same container and send a real PROXY header to the new destination.
	backend, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	headers := make(chan string, 32)
	go func() {
		for {
			c, e := backend.Accept()
			if e != nil {
				return
			}
			go func() {
				defer c.Close()
				c.SetDeadline(time.Now().Add(3 * time.Second))
				reader := bufio.NewReader(c)
				header, e := reader.ReadString('\n')
				if e != nil {
					return
				}
				select {
				case headers <- header:
				default:
				}
				io.Copy(c, reader)
			}()
		}
	}()
	p.TargetPort = backend.Addr().(*net.TCPAddr).Port
	p.ProxyProtocol = true
	p, err = m.SaveStream(revisionOf(t, s), p)
	if err != nil {
		t.Fatal(err)
	}
	apply()
	if dockerCommand(t, "inspect", "--format", "{{.Id}}", name) != id {
		t.Fatal("target change replaced container")
	}
	func() {
		deadline := time.Now().Add(5 * time.Second)
		for {
			echo("tcp")
			select {
			case header := <-headers:
				if !strings.HasPrefix(header, "PROXY TCP4 ") || !strings.HasSuffix(header, "\r\n") {
					t.Fatalf("invalid PROXY header: %q", header)
				}
				return
			default:
			}
			if time.Now().After(deadline) {
				t.Fatal("new destination did not receive PROXY header")
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	// Docker discovers an occupied host port only at start. Failed replacement must restore the old container and revision.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	oldRevision := d.Status(ctx).AppliedRevision
	p.ListenPort = occupied.Addr().(*net.TCPAddr).Port
	p, err = m.SaveStream(revisionOf(t, s), p)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Apply(ctx, revisionOf(t, s)); err == nil {
		t.Fatal("occupied port deployed")
	}
	if dockerCommand(t, "inspect", "--format", "{{.Id}}", name) != id || d.Status(ctx).AppliedRevision != oldRevision {
		t.Fatal("failed deployment lost original container/revision")
	}
	echo("tcp")
	echo("udp")
	// A successful removal recreates the publication set and retains the other protocol.
	if err = m.DeleteStream(revisionOf(t, s), p.ID); err != nil {
		t.Fatal(err)
	}
	apply()
	if dockerCommand(t, "inspect", "--format", "{{.Id}}", name) == id {
		t.Fatal("publication change did not replace container")
	}
	echo("udp")
	c, e := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", hostPort), time.Second)
	if e == nil {
		c.Close()
		t.Fatal("removed TCP port still published")
	}
}
