// Package sshkey is the openvicloud_ssh_key resource: a public key stored in the tenant, picked when a VM is created.
// The API never returns the key itself, only its fingerprint, so the provider compares fingerprints.
package sshkey

import (
	"strings"

	"golang.org/x/crypto/ssh"
)

// Fingerprint is the SHA256 fingerprint of an OpenSSH public key line, as `ssh-keygen -lf` prints it and the API
// stores it.
func Fingerprint(publicKey string) (string, error) {
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(strings.TrimSpace(publicKey)))
	if err != nil {
		return "", err
	}
	return ssh.FingerprintSHA256(key), nil
}
