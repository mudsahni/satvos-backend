package invoice

import (
	"fmt"
	"math"
	"strings"
)

// wordValues maps Indian English number words to their numeric values.
var wordValues = map[string]float64{
	"zero":      0,
	"one":       1,
	"two":       2,
	"three":     3,
	"four":      4,
	"five":      5,
	"six":       6,
	"seven":     7,
	"eight":     8,
	"nine":      9,
	"ten":       10,
	"eleven":    11,
	"twelve":    12,
	"thirteen":  13,
	"fourteen":  14,
	"fifteen":   15,
	"sixteen":   16,
	"seventeen": 17,
	"eighteen":  18,
	"nineteen":  19,
	"twenty":    20,
	"thirty":    30,
	"forty":     40,
	"fifty":     50,
	"sixty":     60,
	"seventy":   70,
	"eighty":    80,
	"ninety":    90,
}

// multiplierValues maps multiplier words to their values.
var multiplierValues = map[string]float64{
	"hundred":  100,
	"thousand": 1000,
	"lakh":     100000,
	"lakhs":    100000,
	"lac":      100000,
	"lacs":     100000,
	"crore":    10000000,
	"crores":   10000000,
}

// ParseIndianAmountWords attempts to parse an Indian English amount string
// like "Two Lakh Eighty Nine Thousand One Hundred Rupees Only" into 289100.00.
// Returns an error if the string cannot be parsed.
func ParseIndianAmountWords(s string) (float64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}

	// Normalize: lowercase, remove common suffixes and punctuation
	normalized := strings.ToLower(s)
	normalized = strings.ReplaceAll(normalized, ",", "")
	normalized = strings.ReplaceAll(normalized, "-", " ")
	normalized = strings.ReplaceAll(normalized, ".", "")

	// Remove common suffixes
	for _, suffix := range []string{" only", " rupees", " rupee", " rs", " inr", " paisa", " paise"} {
		normalized = strings.TrimSuffix(normalized, suffix)
	}
	normalized = strings.TrimSpace(normalized)

	// Split "and" as separator (e.g., "one hundred and fifty")
	normalized = strings.ReplaceAll(normalized, " and ", " ")

	words := strings.Fields(normalized)
	if len(words) == 0 {
		return 0, fmt.Errorf("no parseable words")
	}

	var total float64
	var current float64
	parsed := false

	for _, word := range words {
		if val, ok := wordValues[word]; ok {
			current += val
			parsed = true
		} else if mult, ok := multiplierValues[word]; ok {
			if current == 0 {
				current = 1
			}
			switch {
			case mult >= 100000: // lakh or crore: accumulate to total
				total += current * mult
				current = 0
			case mult == 1000: // thousand
				total += current * mult
				current = 0
			default: // hundred
				current *= mult
			}
			parsed = true
		}
		// Skip unrecognized words (e.g., "and", "of", etc.)
	}

	total += current

	if !parsed {
		return 0, fmt.Errorf("no recognized number words in: %s", s)
	}

	// Round to 2 decimal places
	total = math.Round(total*100) / 100
	return total, nil
}
