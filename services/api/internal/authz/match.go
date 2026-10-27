package authz

// matchWildcard reports whether s matches pattern. A * in the pattern matches any run of
// characters, including none and including "/". Every other character matches itself. This
// is the IAM rule for actions and resources.
func matchWildcard(pattern, s string) bool {
	p, i := 0, 0
	star, mark := -1, 0
	for i < len(s) {
		switch {
		case p < len(pattern) && pattern[p] == '*':
			star, mark = p, i
			p++
		case p < len(pattern) && pattern[p] == s[i]:
			p++
			i++
		case star != -1:
			// Let the last * swallow one more character and retry.
			p = star + 1
			mark++
			i = mark
		default:
			return false
		}
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}
