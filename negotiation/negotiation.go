package negotiation

import (
	"strconv"
	"strings"
)

// SelectQValue selects and returns the best value from the allowed set
// given a header with optional quality values, as you would get for an
// Accept or Accept-Encoding header. The *first* item in allowed is preferred
// if there is a tie. If nothing matches, returns an empty string.
func SelectQValue(header string, allowed []string) string {
	formats := strings.Split(header, ",")
	best := ""
	bestQ := 0.0
	for _, format := range formats {
		parts := strings.Split(format, ";")
		name := strings.Trim(parts[0], " \t")

		found := false
		for _, n := range allowed {
			if n == name {
				found = true
				break
			}
		}

		if !found {
			// Skip formats we don't support.
			continue
		}

		// Default weight to 1 if no value is passed.
		q := 1.0
		if len(parts) > 1 {
			trimmed := strings.Trim(parts[1], " \t")
			if strings.HasPrefix(trimmed, "q=") {
				q, _ = strconv.ParseFloat(trimmed[2:], 64)
			}
		}

		// Prefer the first one if there is a tie.
		if q > bestQ || (q == bestQ && name == allowed[0]) {
			bestQ = q
			best = name
		}
	}

	return best
}
