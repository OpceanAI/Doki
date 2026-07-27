package runtime

import "strings"

// BuildCommand resolves the final argv for a container from the caller's
// entrypoint/cmd and the image's own defaults, applying Docker's override
// rules: an explicit entrypoint replaces the image's, an explicit cmd replaces
// the image's, and a shell-form entrypoint string gets wrapped in `sh -c`.
//
// Both the Docker and the libpod surfaces call this so the two can never drift
// into resolving the same image to different commands.
func BuildCommand(reqEntrypoint, reqCmd []string, img *ImageOCIConfig) []string {
	cmd := reqCmd
	entrypoint := reqEntrypoint

	if len(entrypoint) == 0 && img != nil {
		entrypoint = img.Entrypoint
	}

	// Shell-form entrypoint: a single string carrying spaces or shell operators
	// is not an argv, it is a script line. A leading '[' means the caller
	// already handed us a JSON array, so leave it alone.
	if len(entrypoint) > 0 && len(entrypoint[0]) > 0 && !strings.HasPrefix(entrypoint[0], "[") {
		if strings.ContainsAny(entrypoint[0], " \t") ||
			strings.Contains(entrypoint[0], "&&") ||
			strings.Contains(entrypoint[0], ";") {
			shell := "/bin/sh"
			if img != nil && len(img.Shell) > 0 {
				shell = img.Shell[0]
			}
			entrypoint = []string{shell, "-c", entrypoint[0]}
		}
	}

	if len(entrypoint) > 0 {
		cmd = append(append([]string{}, entrypoint...), cmd...)
	}
	if len(cmd) == 0 && img != nil && len(img.Cmd) > 0 {
		cmd = append(cmd, img.Cmd...)
	}
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}
	return cmd
}
