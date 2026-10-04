//go:build !windows

package playbackasset

func syncPublicationDir(path string) error {
	return syncPublicationParent(path, true)
}
