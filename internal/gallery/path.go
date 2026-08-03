package gallery

import (
	"net/url"
	"path"
)

func urlPathEscape(s string) string {
	return url.PathEscape(s)
}

func urlPath(s string) string {
	return path.Clean("/" + s)
}
