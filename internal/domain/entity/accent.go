package entity

import "unicode"

// AccentMap maps base characters to their accented variants.
// The order determines display order in the accent picker.
var AccentMap = map[rune][]rune{
	'a': {'à', 'á', 'â', 'ä', 'ã', 'å', 'æ'},
	'c': {'ç', 'ć', 'č'},
	'e': {'è', 'é', 'ê', 'ë', 'ę', 'ė'},
	'i': {'ì', 'í', 'î', 'ï', 'į'},
	'n': {'ñ', 'ń'},
	'o': {'ò', 'ó', 'ô', 'ö', 'õ', 'ø', 'œ'},
	's': {'ß', 'ś', 'š'},
	'u': {'ù', 'ú', 'û', 'ü', 'ū'},
	'y': {'ÿ', 'ý'},
}

// HasAccents returns true if the given character has accent variants.
func HasAccents(char rune) bool {
	lower := unicode.ToLower(char)
	_, ok := AccentMap[lower]
	return ok
}

// GetAccents returns the accent variants for a character.
// If uppercase is true, returns uppercase variants.
// Returns nil if the character has no accent variants.
func GetAccents(char rune, uppercase bool) []rune {
	lower := unicode.ToLower(char)
	accents, ok := AccentMap[lower]
	if !ok {
		return nil
	}

	if !uppercase {
		return accents
	}

	// Return uppercase versions
	result := make([]rune, len(accents))
	for i, a := range accents {
		result[i] = unicode.ToUpper(a)
	}
	return result
}
