package cmdutil

// ShareBaseURL resolves the base URL for the shareable preview page.
// Falls back to the public default when no profile / endpoint is
// available, since the preview share URL is useful even without auth.
func ShareBaseURL() string {
	_, url, _, err := Default().Service("share")
	if err != nil || url == "" {
		return "https://beta.lab.shiliu.chat/share"
	}
	return url
}
