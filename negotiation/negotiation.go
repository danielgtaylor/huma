// Package negotiation provides utilities for working with HTTP client-
// driven content negotiation. It provides a zero-allocation utility for
// determining the best content type for the server to encode a response.
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

// SelectQValueFast is a faster version of SelectQValue that does not
// need any dynamic memory allocations.
func SelectQValueFast(header string, allowed []string) string {
	best := ""
	bestQ := 0.0

	name := ""
	start := 0
	end := 0

	for pos, char := range header {
		// Format is like "a; q=0.5, b;q=1.0,c; q=0.3"
		if char == ';' {
			name = header[start : end+1]
			start = pos + 1
			end = start
			continue
		}

		if char == ',' || pos == len(header)-1 {
			q := 1.0
			if char != ',' && char != ' ' && char != '\t' {
				// Update the end if it's not a comma or whitespace (i.e. end of string).
				end = pos
			}
			if name == "" {
				// No name yet means we did not encounter a `;`. Either this is a `,`
				// or the end of the string so whatever we have is the name.
				// Example: "a, b, c"
				name = header[start : end+1]
			} else {
				if len(header) > end+1 {
					if parsed, _ := strconv.ParseFloat(header[start+2:end+1], 64); parsed > 0 {
						q = parsed
					}
				}
			}
			start = pos + 1
			end = start

			found := false
			for _, n := range allowed {
				if n == name {
					found = true
					break
				}
			}

			if !found {
				// Skip formats we don't support.
				name = ""
				continue
			}

			if q > bestQ || (q == bestQ && name == allowed[0]) {
				bestQ = q
				best = name
			}
			name = ""
			continue
		}

		if char != ' ' && char != '\t' {
			// Only advance end if it's not whitespace.
			end = pos
			if header[start] == ' ' || header[start] == '\t' {
				// Trim leading whitespace.
				start = pos
			}
		}
	}

	return best
}
