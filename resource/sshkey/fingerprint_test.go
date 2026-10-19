package sshkey

import "testing"

// The pair below was made with ssh-keygen; the expected value is what `ssh-keygen -lf` prints, the same string the
// API stores as the key's fingerprint.
const publicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIL1eRyvnA7NoD1j8ZEzHCjOwKSd93CJtviepj9+ZXUj9 test"

func TestFingerprint_IsWhatSshKeygenPrints(t *testing.T) {
	got, err := Fingerprint(publicKey)
	if err != nil || got != "SHA256:w7wXsziszcmpDqIzsIehzXWizbM2sbR77M+wGSASbBY" {
		t.Fatalf("fingerprint = %q, err = %v", got, err)
	}
}

func TestFingerprint_NotAKey_IsAnError(t *testing.T) {
	if _, err := Fingerprint("ssh-ed25519 not-base64"); err == nil {
		t.Fatal("a line that is not a public key must be refused")
	}
}
