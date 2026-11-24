package certs

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
)

// Linux distros keep the system CA list in different places.
var linuxStores = []struct {
	dir, file string
	update    []string
}{
	{"/usr/local/share/ca-certificates", "fuck-let-me-work.crt", []string{"update-ca-certificates"}},           // Debian, Ubuntu
	{"/etc/pki/ca-trust/source/anchors", "fuck-let-me-work.pem", []string{"update-ca-trust", "extract"}},       // Fedora, RHEL
	{"/etc/ca-certificates/trust-source/anchors", "fuck-let-me-work.crt", []string{"trust", "extract-compat"}}, // Arch
}

func Trust(certPath string) ([]string, error) {
	pem, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	installed := false
	for _, s := range linuxStores {
		if _, err := os.Stat(s.dir); err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(s.dir, s.file), pem, 0o644); err != nil {
			return nil, err
		}
		if err := run(s.update[0], s.update[1:]...); err != nil {
			return nil, err
		}
		installed = true
		break
	}
	if !installed {
		return nil, errors.New("no known system CA folder found; add " + certPath + " to your system trust by hand")
	}
	return nssEach(func(db, owner string) error {
		return run("sudo", "-u", owner, "certutil", "-d", "sql:"+db, "-A", "-t", "C,,", "-n", CommonName, "-i", certPath)
	}), nil
}

func Untrust(string) ([]string, error) {
	for _, s := range linuxStores {
		p := filepath.Join(s.dir, s.file)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := os.Remove(p); err != nil {
			return nil, err
		}
		if err := run(s.update[0], s.update[1:]...); err != nil {
			return nil, err
		}
	}
	return nssEach(func(db, owner string) error {
		return run("sudo", "-u", owner, "certutil", "-d", "sql:"+db, "-D", "-n", CommonName)
	}), nil
}

// nssEach runs fn on every NSS cert database of the user who ran sudo.
// Chrome and Firefox on Linux use these and ignore the system CA list.
func nssEach(fn func(db, owner string) error) []string {
	owner := os.Getenv("SUDO_USER")
	if owner == "" {
		return []string{"Run install with sudo from your own user so Chrome and Firefox trust the CA too."}
	}
	u, err := user.Lookup(owner)
	if err != nil {
		return []string{err.Error()}
	}
	var dbs []string
	for _, pattern := range []string{
		".pki/nssdb",
		"snap/chromium/current/.pki/nssdb",
		".mozilla/firefox/*",
		"snap/firefox/common/.mozilla/firefox/*",
		".var/app/org.mozilla.firefox/.mozilla/firefox/*",
	} {
		matches, _ := filepath.Glob(filepath.Join(u.HomeDir, pattern, "cert9.db"))
		for _, m := range matches {
			dbs = append(dbs, filepath.Dir(m))
		}
	}
	if len(dbs) == 0 {
		return nil
	}
	if _, err := exec.LookPath("certutil"); err != nil {
		return []string{"Install certutil (Debian/Ubuntu: libnss3-tools, Fedora: nss-tools, Arch: nss) and run install again, so Chrome and Firefox trust the CA."}
	}
	var notes []string
	for _, db := range dbs {
		if err := fn(db, owner); err != nil {
			notes = append(notes, fmt.Sprintf("browser store %s: %v", db, err))
		}
	}
	return notes
}
