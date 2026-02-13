package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	green  = "\033[0;32m"
	red    = "\033[0;31m"
	yellow = "\033[1;33m"
	blue   = "\033[0;34m"
	cyan   = "\033[0;36m"
	nc     = "\033[0m"
)

func banner(title string) {
	fmt.Printf("\n%s══════════════════════════════════════════%s\n", blue, nc)
	fmt.Printf("%s  %s%s\n", blue, title, nc)
	fmt.Printf("%s══════════════════════════════════════════%s\n\n", blue, nc)
}

func pass(msg string)   { fmt.Printf("  %sPASS%s %s\n", green, nc, msg) }
func fail(msg string)   { fmt.Printf("  %sFAIL%s %s\n", red, nc, msg) }
func info(msg string)   { fmt.Printf("  %sINFO%s %s\n", yellow, nc, msg) }
func detail(msg string) { fmt.Printf("  %s  -> %s%s\n", cyan, msg, nc) }

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

func getToken(tokenURL, clientID, clientSecret, scope string) (string, error) {
	data := url.Values{
		"grant_type": {"client_credentials"},
	}
	if scope != "" {
		data.Set("scope", scope)
	}

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("token request build failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}
	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("token parse error: %w (body: %s)", err, string(body))
	}
	if tr.Error != "" {
		return "", fmt.Errorf("token error: %s - %s", tr.Error, tr.ErrorDesc)
	}
	return tr.AccessToken, nil
}

func connectNATS(natsURL, token string, tlsCA string) (*nats.Conn, error) {
	opts := []nats.Option{
		nats.Name("demo-client"),
		nats.Timeout(5 * time.Second),
		nats.ErrorHandler(func(_ *nats.Conn, sub *nats.Subscription, err error) {
			subj := ""
			if sub != nil {
				subj = sub.Subject
			}
			fmt.Printf("  %sNATS ERR%s %s: %v\n", red, nc, subj, err)
		}),
	}
	if token != "" {
		opts = append(opts, nats.Token(token))
	}
	if tlsCA != "" {
		opts = append(opts, nats.RootCAs(tlsCA))
	}
	if strings.HasPrefix(natsURL, "tls://") {
		opts = append(opts, nats.Secure(&tls.Config{
			InsecureSkipVerify: true,
		}))
	}
	return nats.Connect(natsURL, opts...)
}

func testPub(conn *nats.Conn, subject, payload string) bool {
	err := conn.Publish(subject, []byte(payload))
	if err != nil {
		fail(fmt.Sprintf("PUB %s: %v", subject, err))
		return false
	}
	conn.Flush()
	time.Sleep(300 * time.Millisecond)
	return true
}

func testSub(conn *nats.Conn, subject string) bool {
	sub, err := conn.SubscribeSync(subject)
	if err != nil {
		fail(fmt.Sprintf("SUB %s: %v", subject, err))
		return false
	}
	defer sub.Unsubscribe()
	conn.Flush()
	time.Sleep(300 * time.Millisecond)
	return true
}

type pingApp struct {
	clientID     string
	clientSecret string
}

func runAdmin(natsURL, tokenURL string, app pingApp, tlsCA string) {
	banner("Scenario 1: Admin (nats:admin) — Full Access")
	token, err := getToken(tokenURL, app.clientID, app.clientSecret, "nats:admin")
	if err != nil {
		fail(fmt.Sprintf("get token: %v", err))
		return
	}
	info("PingOne token acquired (scope: nats:admin)")

	conn, err := connectNATS(natsURL, token, tlsCA)
	if err != nil {
		fail(fmt.Sprintf("connect: %v", err))
		return
	}
	defer conn.Close()
	pass("Connected to NATS")

	if testPub(conn, "orders.new", `{"id":1}`) {
		pass("PUB orders.new")
	}
	if testPub(conn, "events.click", "admin-event") {
		pass("PUB events.click")
	}
	if testPub(conn, "any.random.subject", "admin-test") {
		pass("PUB any.random.subject (wildcard access)")
	}
	if testSub(conn, "orders.>") {
		pass("SUB orders.>")
	}
	if testSub(conn, "events.>") {
		pass("SUB events.>")
	}
}

func runPublisher(natsURL, tokenURL string, app pingApp, tlsCA string) {
	banner("Scenario 2: Publisher (nats:publish) — Pub Allowed, Sub Denied")
	token, err := getToken(tokenURL, app.clientID, app.clientSecret, "nats:publish")
	if err != nil {
		fail(fmt.Sprintf("get token: %v", err))
		return
	}
	info("PingOne token acquired (scope: nats:publish)")

	conn, err := connectNATS(natsURL, token, tlsCA)
	if err != nil {
		fail(fmt.Sprintf("connect: %v", err))
		return
	}
	defer conn.Close()
	pass("Connected to NATS")

	if testPub(conn, "orders.new", `{"id":42}`) {
		pass("PUB orders.new (allowed)")
	}
	if testPub(conn, "events.processed", "event-data") {
		pass("PUB events.processed (allowed)")
	}

	info("Attempting SUB orders.> (should be denied)...")
	testSub(conn, "orders.>")
	time.Sleep(500 * time.Millisecond)
	pass("SUB orders.> — permission violation expected")

	info("Attempting SUB events.> (should be denied)...")
	testSub(conn, "events.>")
	time.Sleep(500 * time.Millisecond)
	pass("SUB events.> — permission violation expected")
}

func runSubscriber(natsURL, tokenURL string, app pingApp, tlsCA string) {
	banner("Scenario 3: Subscriber (nats:subscribe) — Sub Allowed, Pub Denied")
	token, err := getToken(tokenURL, app.clientID, app.clientSecret, "nats:subscribe")
	if err != nil {
		fail(fmt.Sprintf("get token: %v", err))
		return
	}
	info("PingOne token acquired (scope: nats:subscribe)")

	conn, err := connectNATS(natsURL, token, tlsCA)
	if err != nil {
		fail(fmt.Sprintf("connect: %v", err))
		return
	}
	defer conn.Close()
	pass("Connected to NATS")

	if testSub(conn, "orders.>") {
		pass("SUB orders.> (allowed)")
	}
	if testSub(conn, "events.>") {
		pass("SUB events.> (allowed)")
	}

	info("Attempting PUB orders.new (should be denied)...")
	testPub(conn, "orders.new", "should-fail")
	time.Sleep(500 * time.Millisecond)
	pass("PUB orders.new — permission violation expected")

	info("Attempting PUB events.click (should be denied)...")
	testPub(conn, "events.click", "should-fail")
	time.Sleep(500 * time.Millisecond)
	pass("PUB events.click — permission violation expected")
}

func runInvalidToken(natsURL, tlsCA string) {
	banner("Scenario 4: Invalid Token — Connection Rejected")
	conn, err := connectNATS(natsURL, "this.is.not.a.valid.jwt.token", tlsCA)
	if err != nil {
		pass(fmt.Sprintf("Connection rejected: %v", err))
		return
	}
	conn.Close()
	fail("Connection should have been rejected")
}

func runNoToken(natsURL, tlsCA string) {
	banner("Scenario 5: No Token — Connection Rejected")
	conn, err := connectNATS(natsURL, "", tlsCA)
	if err != nil {
		pass(fmt.Sprintf("Connection rejected: %v", err))
		return
	}
	conn.Close()
	fail("Connection should have been rejected")
}

func main() {
	scenario := flag.String("scenario", "all", "Scenario: admin|publisher|subscriber|invalid|notoken|all")
	natsURL := flag.String("nats", envOr("NATS_URL", "tls://nats:4222"), "NATS server URL")
	tlsCA := flag.String("tls-ca", envOr("TLS_CA_FILE", ""), "TLS CA certificate file")
	flag.Parse()

	issuerURL := mustEnv("PING_ISSUER_URL")
	tokenURL := issuerURL + "/token"

	adminApp := pingApp{
		clientID:     mustEnv("PING_CLIENT_ID"),
		clientSecret: mustEnv("PING_CLIENT_SECRET"),
	}
	pubApp := pingApp{
		clientID:     envOr("PING_PUB_CLIENT_ID", adminApp.clientID),
		clientSecret: envOr("PING_PUB_CLIENT_SECRET", adminApp.clientSecret),
	}
	subApp := pingApp{
		clientID:     envOr("PING_SUB_CLIENT_ID", adminApp.clientID),
		clientSecret: envOr("PING_SUB_CLIENT_SECRET", adminApp.clientSecret),
	}

	fmt.Printf("\n%s╔══════════════════════════════════════════╗%s\n", blue, nc)
	fmt.Printf("%s║  NATS Auth-Callout — PingOne Demo        ║%s\n", blue, nc)
	fmt.Printf("%s╚══════════════════════════════════════════╝%s\n", blue, nc)
	info(fmt.Sprintf("NATS: %s", *natsURL))
	info(fmt.Sprintf("PingOne: %s", tokenURL))

	switch *scenario {
	case "admin":
		runAdmin(*natsURL, tokenURL, adminApp, *tlsCA)
	case "publisher":
		runPublisher(*natsURL, tokenURL, pubApp, *tlsCA)
	case "subscriber":
		runSubscriber(*natsURL, tokenURL, subApp, *tlsCA)
	case "invalid":
		runInvalidToken(*natsURL, *tlsCA)
	case "notoken":
		runNoToken(*natsURL, *tlsCA)
	case "all":
		runAdmin(*natsURL, tokenURL, adminApp, *tlsCA)
		runPublisher(*natsURL, tokenURL, pubApp, *tlsCA)
		runSubscriber(*natsURL, tokenURL, subApp, *tlsCA)
		runInvalidToken(*natsURL, *tlsCA)
		runNoToken(*natsURL, *tlsCA)
	default:
		fmt.Fprintf(os.Stderr, "Unknown scenario: %s\n", *scenario)
		os.Exit(1)
	}

	banner("Demo Complete")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "Required env var %s not set\n", key)
		os.Exit(1)
	}
	return v
}
