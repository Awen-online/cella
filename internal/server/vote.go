package server

import (
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/Awen-online/cella/internal/cardano"
)

// handleVotePrepare returns the exact text a delegate must sign to record the
// position they have filled in. The browser does not compose this itself: the
// server is the only thing that decides what a signature means, so it is also
// the only thing that writes the words.
func (s *Server) handleVotePrepare(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessionID, ok := s.member(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not signed in"})
		return
	}
	if !s.checkCSRF(r, sessionID) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "bad CSRF token"})
		return
	}

	a, ok, err := s.db.ActionBySlug(r.FormValue("slug"))
	if err != nil || !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown action"})
		return
	}
	vote := r.FormValue("vote")
	if !validVote(vote) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid vote"})
		return
	}

	msg := voteMessage(s.body.Name, a.ProposalID, vote, r.FormValue("rationale"))
	writeJSON(w, http.StatusOK, map[string]string{
		"message": msg,
		"hex":     hex.EncodeToString([]byte(msg)),
	})
}

// handleCastVote records the signed-in delegate's internal position + rationale
// on an action (stored in the shared instance so co-members see it).
//
// When the delegate's wallet signed the position, the signature is verified
// against a message this handler rebuilds from the vote it is about to store —
// never against anything the browser asserts was signed. That is what stops a
// delegate being shown one position and having another recorded, and it is why
// a signature cannot be lifted from one action and replayed onto another.
func (s *Server) handleCastVote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sessionID, ok := s.member(r)
	if !ok {
		http.Redirect(w, r, "/enter", http.StatusFound)
		return
	}
	if !s.checkCSRF(r, sessionID) {
		http.Error(w, "bad or missing CSRF token", http.StatusForbidden)
		return
	}
	member := strings.TrimSuffix(sessionID, " (demo)")

	slug := r.FormValue("slug")
	vote := r.FormValue("vote")
	rationale := strings.TrimSpace(r.FormValue("rationale"))
	if !validVote(vote) {
		http.Redirect(w, r, "/action/"+slug, http.StatusFound)
		return
	}

	a, ok, err := s.db.ActionBySlug(slug)
	if err != nil || !ok {
		http.NotFound(w, r)
		return
	}

	sig, key := r.FormValue("signature"), r.FormValue("key")
	if sig != "" {
		if err := s.verifyVoteSignature(member, a.ProposalID, vote, rationale, sig, key); err != nil {
			http.Error(w, "signature rejected: "+err.Error(), http.StatusForbidden)
			return
		}
	}

	if err := s.db.UpsertMemberVote(a.ProposalID, member, vote, rationale, sig, key); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/action/"+a.Slug()+"#your-position", http.StatusFound)
}

// verifyVoteSignature checks that sig is the delegate's signature over exactly
// the position being recorded.
func (s *Server) verifyVoteSignature(member, proposalID, vote, rationale, sigHex, keyHex string) error {
	payload, pub, ok, err := verifyCOSESign1(sigHex, keyHex)
	if err != nil {
		return err
	}
	if !ok {
		return errBadSignature
	}

	// The signed bytes must be the message for *this* position, rebuilt here.
	// A signature over any other action, vote, or rationale will not match.
	want := voteMessage(s.body.Name, proposalID, vote, rationale)
	if !subtleEqual(payload, []byte(want)) {
		return errWrongMessage
	}

	// And the key that signed must belong to the delegate whose session this is
	// — otherwise one delegate could sign a position into another's name.
	signer, ok := s.body.ByCredential(hex.EncodeToString(cardano.KeyHash(pub)))
	if !ok {
		return errNotOnRoster
	}
	if signer.Name != member {
		return errWrongDelegate
	}
	return nil
}

func validVote(v string) bool { return v == "Yes" || v == "No" || v == "Abstain" }

const (
	errBadSignature  = constErr("the signature did not verify")
	errWrongMessage  = constErr("the signature does not cover this position")
	errNotOnRoster   = constErr("the signing wallet is not on the delegate roster")
	errWrongDelegate = constErr("the signing wallet belongs to a different delegate")
)
