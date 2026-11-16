// Package secrets encrypts secret environment values with AES-GCM. The key
// lives in the OS keyring, never next to the database; an unavailable
// keyring is reported as KEYRING_UNAVAILABLE.
package secrets
