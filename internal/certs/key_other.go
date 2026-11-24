//go:build !windows

package certs

// restrictToAdmins does nothing here: the 0600 mode already limits the file to its owner (root).
func restrictToAdmins(string) error { return nil }
