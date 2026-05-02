//go:build !windows && !darwin && !linux

package notify

func newPlatformSender() (platformSender, bool) {
	return nil, false
}
