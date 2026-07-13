package vault

import "testing"

func TestRoundTripAndRandomNonce(t *testing.T) {
	c, e := New("0123456789abcdef")
	if e != nil {
		t.Fatal(e)
	}
	a, _ := c.Seal([]byte("secret"))
	b, _ := c.Seal([]byte("secret"))
	if string(a) == string(b) {
		t.Fatal("ciphertexts unexpectedly equal")
	}
	plain, e := c.Open(a)
	if e != nil || string(plain) != "secret" {
		t.Fatalf("round trip: %q %v", plain, e)
	}
}
