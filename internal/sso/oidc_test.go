package sso

import (
	"context"
	"testing"
)

func TestDisabledAndState(t *testing.T) {
	provider, err := New(context.Background(), "", "", "", "")
	if err != nil || provider != nil {
		t.Fatalf("disabled provider: %v %v", provider, err)
	}
	a, err := State()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := State()
	if len(a) < 40 || a == b {
		t.Fatal("OIDC state is not suitably random")
	}
}
