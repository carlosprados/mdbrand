// Package suggest names the key someone probably meant. A YAML decoder drops
// a field it does not know in silence, so every place that refuses unknown
// keys instead wants the same "did you mean": one nearest candidate or none.
package suggest

// Closest is the candidate within two edits of k, if there is exactly one
// nearest; a guess between two is no help.
func Closest(k string, known []string) string {
	best, bestD, tie := "", 3, false
	for _, c := range known {
		switch d := editDistance(k, c); {
		case d < bestD:
			best, bestD, tie = c, d, false
		case d == bestD:
			tie = true
		}
	}
	if tie {
		return ""
	}
	return best
}

func editDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur := make([]int, len(rb)+1)
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(rb)]
}
