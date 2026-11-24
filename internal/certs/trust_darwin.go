package certs

const systemKeychain = "/Library/Keychains/System.keychain"

const firefoxNote = "Firefox: if blocked sites show a certificate warning, set security.enterprise_roots.enabled to true in about:config."

func Trust(certPath string) ([]string, error) {
	return []string{firefoxNote}, run("security", "add-trusted-cert", "-d", "-r", "trustRoot", "-k", systemKeychain, certPath)
}

func Untrust(string) ([]string, error) {
	return nil, run("security", "delete-certificate", "-c", CommonName, systemKeychain)
}
