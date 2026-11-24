package salutejazz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openlibrecommunity/olcrtc/internal/auth"
)

func TestIssueRunsPreconnectAndReturnsConnectorCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/room/abc123/preconnect" {
			t.Errorf("path %q", r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["password"] != "passw0rd" {
			t.Errorf("password %q", body["password"])
		}
		if r.Header.Get("Origin") != "https://salutejazz.ru" {
			t.Errorf("origin %q", r.Header.Get("Origin"))
		}
		_, _ = w.Write([]byte(`{"connectorUrl":"wss://fake/connector","roomTitle":"t","participantRole":"MEMBER"}`))
	}))
	defer srv.Close()

	p := Provider{apiBase: srv.URL}
	creds, err := p.Issue(context.Background(), auth.Config{RoomURL: "abc123:passw0rd", Name: "n"})
	if err != nil {
		t.Fatal(err)
	}
	if creds.URL != "wss://fake/connector" || creds.Token != "passw0rd" || creds.Extra["roomID"] != "abc123" {
		t.Fatalf("creds %+v", creds)
	}
}

func TestIssueRejectsMalformedRoomReferences(t *testing.T) {
	p := Provider{}
	// Mixed case is allowed now (web-UI rooms carry "?psw=" values), so the
	// rejected shapes are the structural ones: missing colon, empty halves,
	// an extra colon, a space or a shell-hostile character.
	for _, bad := range []string{"", "any", "abc123", "abc123:", ":passw0rd", "abc:pass:extra", "abc123:pass word", "abc123:pw-1234"} {
		if _, err := p.Issue(context.Background(), auth.Config{RoomURL: bad}); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestCreateRoomReturnsCodeAndPassword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/room/create-meeting" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"roomId":"zzz999","password":"pw123456","url":"https://salutejazz.ru/zzz999?psw=x"}`))
	}))
	defer srv.Close()

	p := Provider{apiBase: srv.URL}
	room, err := p.CreateRoom(context.Background(), auth.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if room != "zzz999:pw123456" {
		t.Fatalf("room %q", room)
	}
}

// TestIssueErrorNeverContainsPassword guards the credential-leak fix: a
// malformed room reference must be refused without echoing the password
// half anywhere in the error text, because that error is logged by callers
// several layers up (engineconn -> builtin -> Logf).
func TestIssueErrorNeverContainsPassword(t *testing.T) {
	p := Provider{}
	_, err := p.Issue(context.Background(), auth.Config{RoomURL: "abc123:pw-1234"})
	if err == nil {
		t.Fatal("expected an error for a malformed password")
	}
	if strings.Contains(err.Error(), "pw-1234") {
		t.Fatalf("error leaked the password: %v", err)
	}
}

// TestCreateRoomRejectsMalformedReply guards against trusting the server's
// create-meeting reply blindly: a malformed roomId must fail CreateRoom
// itself (not surface later as a confusing Issue error), and the resulting
// error must not echo the password either.
func TestCreateRoomRejectsMalformedReply(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"roomId":"zzz999","password":"pw-123456","url":"https://salutejazz.ru/zzz999?psw=x"}`))
	}))
	defer srv.Close()

	p := Provider{apiBase: srv.URL}
	room, err := p.CreateRoom(context.Background(), auth.Config{})
	if err == nil {
		t.Fatalf("expected an error for a malformed password, got room %q", room)
	}
	if strings.Contains(err.Error(), "pw-123456") {
		t.Fatalf("error leaked the password: %v", err)
	}
}
