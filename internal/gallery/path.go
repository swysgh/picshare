package gallery

import (
	"net/url"
)

// urlQueryEscape escapes a value destined for a URL query component
// (?p=…). QueryEscape (not PathEscape) is required: PathEscape leaves '+'
// unescaped, and a '+' in a query component decodes to a space, breaking
// file names that contain it.
func urlPathEscape(s string) string {
	return url.QueryEscape(s)
}
