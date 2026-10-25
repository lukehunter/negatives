//go:build !windows

package archivesync

// killWithUs does nothing outside Windows, where robocopy doesn't exist.
func killWithUs(int) {}
