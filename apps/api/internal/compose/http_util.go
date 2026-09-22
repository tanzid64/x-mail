package compose

import (
	"io"
	"net/url"
)

// copyTo streams a stored object to the response.
func copyTo(dst io.Writer, src io.Reader) (int64, error) {
	return io.Copy(dst, src)
}

// escapeFilename percent-encodes a filename for the RFC 5987 form of
// Content-Disposition, which is what carries non-ASCII names safely.
func escapeFilename(name string) string {
	return url.PathEscape(name)
}
