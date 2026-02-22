package normalize

import (
	"regexp"
	"strings"
)

var multiSpaceRe = regexp.MustCompile(`\s{2,}`)

// suffixReplacements maps legal suffix forms to their canonical abbreviation.
// Order matters: longest matches must come first to avoid partial replacement.
var suffixReplacements = []struct {
	from string
	to   string
}{
	{"PRIVATE LIMITED", "PVT LTD"},
	{"PVT LIMITED", "PVT LTD"},
	{"PRIVATE LTD", "PVT LTD"},
	{"LIMITED", "LTD"},
	{"CORPORATION", "CORP"},
	{"INCORPORATED", "INC"},
	{"COMPANY", "CO"},
	{"AND", "&"},
}

// CompanyName normalizes a company name to a canonical UPPER CASE form.
// Rules applied in order:
//  1. Trim whitespace
//  2. Collapse multiple spaces to single
//  3. Convert to UPPER CASE
//  4. Remove dots (e.g., "P.V.T." → "PVT")
//  5. Normalize legal suffixes (longest match first):
//     PRIVATE LIMITED → PVT LTD, LIMITED → LTD, CORPORATION → CORP,
//     INCORPORATED → INC, COMPANY → CO, AND → &
//  6. Re-trim and re-collapse spaces
func CompanyName(name string) string {
	if name == "" {
		return ""
	}

	// 1. Trim whitespace
	s := strings.TrimSpace(name)
	if s == "" {
		return ""
	}

	// 2. Collapse multiple spaces
	s = multiSpaceRe.ReplaceAllString(s, " ")

	// 3. UPPER CASE
	s = strings.ToUpper(s)

	// 4. Remove dots
	s = strings.ReplaceAll(s, ".", "")

	// 5. Normalize legal suffixes using whole-word replacement
	for _, r := range suffixReplacements {
		s = replaceWholeWord(s, r.from, r.to)
	}

	// 6. Re-trim and re-collapse
	s = strings.TrimSpace(s)
	s = multiSpaceRe.ReplaceAllString(s, " ")

	return s
}

// replaceWholeWord replaces all whole-word occurrences of old with new_ in s.
// A "whole word" is bounded by start/end of string or spaces.
func replaceWholeWord(s, old, new_ string) string {
	words := strings.Fields(s)
	oldWords := strings.Fields(old)
	newWords := strings.Fields(new_)
	oldLen := len(oldWords)

	if oldLen == 0 || len(words) < oldLen {
		return s
	}

	var result []string
	i := 0
	for i <= len(words)-oldLen {
		match := true
		for j := 0; j < oldLen; j++ {
			if words[i+j] != oldWords[j] {
				match = false
				break
			}
		}
		if match {
			result = append(result, newWords...)
			i += oldLen
		} else {
			result = append(result, words[i])
			i++
		}
	}
	// Append remaining words
	for ; i < len(words); i++ {
		result = append(result, words[i])
	}

	return strings.Join(result, " ")
}
