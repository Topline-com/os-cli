package output

import "regexp"

var (
	emailRe  = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	phoneRe  = regexp.MustCompile(`(?:\+?1[\s.\-]*)?(?:\(?\d{3}\)?[\s.\-]*)\d{3}[\s.\-]*\d{4}`)
	bearerRe = regexp.MustCompile(`(?i)Bearer\s+[^\s"']+`)
	pitRe    = regexp.MustCompile(`(?i)(TOPLINE_PIT\s*=\s*)[^\s"']+`)
)

func MaskPII(s string) string {
	s = bearerRe.ReplaceAllString(s, "Bearer [REDACTED]")
	s = pitRe.ReplaceAllString(s, "${1}[REDACTED]")
	s = emailRe.ReplaceAllString(s, "[EMAIL]")
	s = phoneRe.ReplaceAllString(s, "[PHONE]")
	return s
}
