package server

import "strings"

// Every governance action is judged against the Constitution, but not every
// article bears on every action. A treasury withdrawal lives or dies by the
// guardrails; a committee update by Article III. Pointing a delegate at the
// articles that actually govern the action in front of them is the difference
// between a Constitution they cite and one they scroll past.
//
// This is a starting point for reading, not a limit on it. The committee is
// free — and obliged — to find the article Cella did not think to name.

// Article is a deep link into the Constitution viewer.
type Article struct {
	Title string
	ID    string // heading anchor in the rendered Constitution
}

// Alignment is the set of articles that most directly govern an action type.
type Alignment struct {
	Lead     string
	Articles []Article
}

// The anchors goldmark derives from the current revision's headings.
var (
	artI   = Article{"Article I — Tenets & Guardrails", "article-i-cardano-blockchain-tenets-and-guardrails"}
	artII  = Article{"Article II — Community & Governance", "article-ii-community-and-governance"}
	artIII = Article{"Article III — Constitutional Committee", "article-iii-constitutional-committee"}
	artIV  = Article{"Article IV — Amendment Process", "article-iv-amendment-process"}
	appI   = Article{"Appendix I — Guardrails", "appendix-i-cardano-blockchain-guardrails"}
)

// alignments maps an action type to the articles that govern it. The key is
// normalised so that both Koios's CamelCase ("TreasuryWithdrawals") and any
// spaced rendering ("Treasury Withdrawals") resolve to the same entry.
var alignments = map[string]Alignment{
	"treasurywithdrawals": {
		Lead:     "Treasury actions are bound by the guardrails: net change limits, and discipline about who may receive funds and on what terms.",
		Articles: []Article{artI, appI},
	},
	"parameterchange": {
		Lead:     "Protocol parameters are fenced by the guardrails, and values outside the permitted range are rejected by the guardrails script regardless of how the vote goes.",
		Articles: []Article{artI, appI},
	},
	"hardforkinitiation": {
		Lead:     "A hard fork changes the ledger rules themselves and must clear the highest thresholds in the framework.",
		Articles: []Article{artI, artII},
	},
	"newcommittee": {
		Lead:     "Committee composition, quorum, and term length are governed by Article III — including the committee's own removal.",
		Articles: []Article{artIII},
	},
	"updatecommittee": {
		Lead:     "Committee composition, quorum, and term length are governed by Article III — including the committee's own removal.",
		Articles: []Article{artIII},
	},
	"noconfidence": {
		Lead:     "A motion of no confidence dissolves the committee under the Article III provisions, and stalls every action that needs a committee vote until a new one is seated.",
		Articles: []Article{artIII},
	},
	"newconstitution": {
		Lead:     "Amending the Constitution itself is governed by Article IV. Read the proposed text before voting; it becomes the yardstick for every action after it.",
		Articles: []Article{artIV},
	},
	"infoaction": {
		Lead:     "An information action records the collective position of the voting bodies. Nothing transacts, but the record stands.",
		Articles: []Article{artII},
	},
}

// alignmentFor returns the articles governing an action type, if Cella knows of
// any. When it does not, it says nothing rather than pointing the committee at
// an article that may have no bearing on the action at all.
func alignmentFor(actionType string) (Alignment, bool) {
	key := strings.ToLower(strings.ReplaceAll(actionType, " ", ""))
	a, ok := alignments[key]
	return a, ok
}
