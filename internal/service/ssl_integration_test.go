package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/miekg/dns"
	"web-panel/internal/domain"
	"web-panel/internal/onepanel/model"
	"web-panel/internal/openresty"
	"web-panel/internal/store"
)

// Opt-in real ACME protocol test: Pebble validates HTTP-01 through the actual
// managed OpenResty container and DNS-01 against an isolated authoritative fixture.
// It never registers an account or changes a DNS record at a public CA/provider.
func TestSSLACMEIntegration(t *testing.T) {
	if os.Getenv("RESTY_ACME_INTEGRATION") != "1" {
		t.Skip("set RESTY_ACME_INTEGRATION=1; requires Docker and the pinned Pebble image")
	}
	const image = "ghcr.io/letsencrypt/pebble@sha256:0c81f05db371f436c97fb894bd6d2ab8fc9c8e2ce40f8bef8a8e15941bcd75ef"
	if out, e := exec.Command("docker", "image", "inspect", image).CombinedOutput(); e != nil {
		t.Fatalf("pull %s before this test: %v %s", image, e, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	run := func(args ...string) []byte {
		t.Helper()
		out, e := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		if e != nil {
			t.Fatalf("docker %v: %v %s", args, e, out)
		}
		return out
	}
	root := t.TempDir()
	pebbleName := "resty-acme-" + domain.NewID()[:10]
	run("create", "--name", pebbleName+"-files", image)
	caPath := filepath.Join(root, "pebble-ca.pem")
	run("cp", pebbleName+"-files:/test/certs/pebble.minica.pem", caPath)
	run("rm", pebbleName+"-files")
	t.Setenv("LEGO_CA_CERTIFICATES", caPath)
	t.Setenv("LEGO_CA_SERVER_NAME", "localhost")
	// This DNS server answers only the local test's names. It never updates external DNS.
	udp, e := net.ListenPacket("udp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	var txtMu sync.RWMutex
	txt := map[string]string{}
	dnsServer := &dns.Server{PacketConn: udp, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, q *dns.Msg) {
		reply := new(dns.Msg)
		reply.SetReply(q)
		reply.Authoritative = true
		for _, question := range q.Question {
			switch question.Qtype {
			case dns.TypeA:
				reply.Answer = append(reply.Answer, &dns.A{Hdr: dns.RR_Header{Name: question.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 1}, A: net.IPv4(127, 0, 0, 1)})
			case dns.TypeTXT:
				txtMu.RLock()
				value := txt[question.Name]
				txtMu.RUnlock()
				if value != "" {
					reply.Answer = append(reply.Answer, &dns.TXT{Hdr: dns.RR_Header{Name: question.Name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 1}, Txt: []string{value}})
				}
			}
		}
		w.WriteMsg(reply)
	})}
	tcp, e := net.Listen("tcp", udp.LocalAddr().String())
	if e != nil {
		t.Fatal(e)
	}
	tcpDNS := &dns.Server{Listener: tcp, Handler: dnsServer.Handler}
	go tcpDNS.ActivateAndServe()
	t.Cleanup(func() { tcpDNS.Shutdown() })
	go dnsServer.ActivateAndServe()
	t.Cleanup(func() { dnsServer.Shutdown() })
	s, e := store.Open(filepath.Join(root, "manager"))
	if e != nil {
		t.Fatal(e)
	}
	webPort := freePort(t)
	acmePort := freePort(t)
	managementPort := freePort(t)
	webNumber, _ := strconv.Atoi(webPort)
	config := map[string]any{"pebble": map[string]any{"listenAddress": "127.0.0.1:" + acmePort, "managementListenAddress": "127.0.0.1:" + managementPort, "certificate": "test/certs/localhost/cert.pem", "privateKey": "test/certs/localhost/key.pem", "httpPort": webNumber, "tlsPort": 5001, "externalAccountBindingRequired": false, "retryAfter": map[string]int{"authz": 1, "order": 1}}}
	blob, _ := json.Marshal(config)
	configPath := filepath.Join(root, "pebble.json")
	if e = os.WriteFile(configPath, blob, 0600); e != nil {
		t.Fatal(e)
	}
	run("run", "-d", "--rm", "--network", "host", "--name", pebbleName, "-e", "PEBBLE_VA_NOSLEEP=1", "-e", "PEBBLE_AUTHZREUSE=0", "--mount", "type=bind,src="+configPath+",dst=/test/custom.json,readonly", image, "-config", "/test/custom.json", "-dnsserver", udp.LocalAddr().String())
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", pebbleName).Run() })
	caPEM, _ := os.ReadFile(caPath)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}}
	directory := "https://localhost:" + acmePort + "/dir"
	ready := false
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
		res, e := client.Get(directory)
		if e == nil {
			res.Body.Close()
			if res.StatusCode == 200 {
				ready = true
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("Pebble not ready: %s", run("logs", pebbleName))
	}
	engine := &openresty.Docker{Root: s.Root, Image: "1panel/openresty:1.27.1.2-5-1-focal", Container: pebbleName + "-web", HTTPBind: "127.0.0.1:" + webPort, HTTPSBind: "127.0.0.1:" + freePort(t)}
	t.Cleanup(func() { exec.Command("docker", "rm", "-f", engine.Container).Run() })
	m, e := New(s, engine)
	if e != nil {
		t.Fatal(e)
	}
	if e = m.Apply(ctx, revisionOf(t, s)); e != nil {
		t.Fatal(e)
	}
	if e = m.SSLRegister(ctx, model.WebsiteAcmeAccount{Email: "acme@example.test", Type: "custom", CaDirURL: directory, KeyType: "EC256"}); e != nil {
		t.Fatal(e)
	}
	st, _ := s.Load()
	account := st.SSL.Accounts[0]
	waitReady := func(id uint) {
		t.Helper()
		for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); {
			v, e := m.SSLGet(id)
			if e != nil {
				t.Fatal(e)
			}
			if v.Status == "ready" {
				return
			}
			if v.Status == "error" {
				logs, _ := m.SSLLog(id)
				t.Fatalf("ACME failed: %s\n%v", v.Message, logs)
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("ACME job timed out")
	}
	id, e := m.SSLSave(SSLCommand{WebsiteSSL: model.WebsiteSSL{PrimaryDomain: "http.acme.example.test", Provider: "http", AcmeAccountID: account.ID, KeyType: "EC256"}}, true)
	if e != nil {
		t.Fatal(e)
	}
	if e = m.SSLObtain(id); e != nil {
		t.Fatal(e)
	}
	waitReady(id)
	first, _ := m.SSLGet(id)
	if e = m.SSLObtain(id); e != nil {
		t.Fatal(e)
	}
	waitReady(id)
	renewed, _ := m.SSLGet(id)
	if first.Pem == renewed.Pem {
		t.Fatal("renewal returned old certificate")
	}
	manualID, e := m.SSLSave(SSLCommand{WebsiteSSL: model.WebsiteSSL{PrimaryDomain: "manual.acme.example.test", Provider: "dnsManual", Nameserver1: udp.LocalAddr().String(), AcmeAccountID: account.ID, KeyType: "EC256"}}, true)
	if e != nil {
		t.Fatal(e)
	}
	records, e := m.SSLResolve(ctx, manualID)
	if e != nil {
		t.Fatal(e)
	}
	if len(records) != 1 {
		t.Fatalf("DNS records: %v", records)
	}
	txtMu.Lock()
	for _, r := range records {
		txt[dns.Fqdn(r["resolve"])] = r["value"]
	}
	txtMu.Unlock()
	if e = m.SSLObtain(manualID); e != nil {
		t.Fatal(e)
	}
	waitReady(manualID)
	if e = m.SSLDelete(ctx, []uint{id, manualID}); e != nil {
		t.Fatal(e)
	}
	st, _ = s.Load()
	if len(st.SSL.Records) != 0 || len(st.Certificates) != 0 {
		t.Fatal("revoked ACME certificates left behind")
	}
	if e = m.SSLDeleteAccount(account.ID, false); e != nil {
		t.Fatal(e)
	}
	t.Logf("HTTP-01, renewal, manual DNS-01 and ACME revocation passed against %s", image)
}
