package server

import "strings"

// CC hot credential -> readable committee-member name. The votes carry only the
// cc_hot credential; this map gives them human names on the roster. Extend it
// with the committee members relevant to your instance.
var ccNames = map[string]string{}

// ccNamePrefixes matches by credential prefix (handy when only a prefix is on
// hand). Demo: Cardano Curia (this committee).
var ccNamePrefixes = map[string]string{
	"cc_hot1qwz0aw5583t56fvcg": "Cardano Curia",
}

// ccMemberName returns the readable name for a cc_hot credential, or "".
func ccMemberName(voterID string) string {
	if n, ok := ccNames[voterID]; ok {
		return n
	}
	for pfx, n := range ccNamePrefixes {
		if strings.HasPrefix(voterID, pfx) {
			return n
		}
	}
	return ""
}
