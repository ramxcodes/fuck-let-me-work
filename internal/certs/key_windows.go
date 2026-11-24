package certs

// restrictToAdmins limits a file to SYSTEM (S-1-5-18) and Administrators (S-1-5-32-544).
// File mode bits do nothing on Windows, and Program Files lets all users read.
func restrictToAdmins(path string) error {
	return run("icacls", path, "/inheritance:r", "/grant:r", "*S-1-5-18:F", "*S-1-5-32-544:F")
}
