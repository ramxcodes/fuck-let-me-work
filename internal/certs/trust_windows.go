package certs

func Trust(certPath string) ([]string, error) {
	return nil, run("certutil", "-addstore", "-f", "ROOT", certPath)
}

func Untrust(string) ([]string, error) {
	return nil, run("certutil", "-delstore", "ROOT", CommonName)
}
