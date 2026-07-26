package api

import (
	"testing"
	"time"

	"github.com/TerminalAddict/golive-nms/internal/store"
)

func TestLoginCookieIsSessionOnlyByDefault(t *testing.T) {
	cookie := loginCookie("token", false)

	if cookie.MaxAge != 0 {
		t.Fatalf("session cookie MaxAge = %d, want 0", cookie.MaxAge)
	}
	if !cookie.Expires.IsZero() {
		t.Fatalf("session cookie expiry = %v, want zero", cookie.Expires)
	}
	if !cookie.HttpOnly || !cookie.Secure {
		t.Fatal("login cookie must be HttpOnly and Secure")
	}
}

func TestRememberedLoginCookieLastsThirtyDays(t *testing.T) {
	before := time.Now().Add(store.RememberedSessionLifetime)
	cookie := loginCookie("token", true)
	after := time.Now().Add(store.RememberedSessionLifetime)

	if cookie.MaxAge != int(store.RememberedSessionLifetime.Seconds()) {
		t.Fatalf("remembered cookie MaxAge = %d", cookie.MaxAge)
	}
	if cookie.Expires.Before(before) || cookie.Expires.After(after) {
		t.Fatalf("remembered cookie expiry %v is outside expected range", cookie.Expires)
	}
}
