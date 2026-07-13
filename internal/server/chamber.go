package server

import (
	"net/http"
	"strings"
)

// Deliberation is not only voting. Before a committee can take a position, its
// delegates need to flag concerns to each other and think in private — and the
// two are different acts. A flag is addressed to the chamber; a draft is
// addressed to nobody.

// Flags a delegate can raise. Kept deliberately few: a flag that means anything
// specific is a flag nobody uses.
var flagKinds = map[string]string{
	"discuss": "Needs discussion",
	"ready":   "Ready to vote",
	"blocked": "Blocked",
}

// FlagView is one flag kind and who has raised it.
type FlagView struct {
	Kind    string
	Label   string
	Members []string // delegates who raised it
	Mine    bool     // the signed-in delegate is one of them
}

// Raised reports whether anyone has raised this flag.
func (f FlagView) Raised() bool { return len(f.Members) > 0 }

// flagViews assembles the chamber's flags on an action for display.
func (s *Server) flagViews(proposalID, you string) ([]FlagView, error) {
	raised, err := s.db.FlagsFor(proposalID)
	if err != nil {
		return nil, err
	}

	// A fixed order, so the buttons do not move around between page loads.
	order := []string{"discuss", "ready", "blocked"}
	out := make([]FlagView, 0, len(order))
	for _, kind := range order {
		v := FlagView{Kind: kind, Label: flagKinds[kind]}
		for _, f := range raised[kind] {
			v.Members = append(v.Members, f.Member)
			if f.Member == you {
				v.Mine = true
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// handleFlag toggles the signed-in delegate's flag on an action.
//
// A delegate toggles only their own. Lowering a colleague's flag would silence
// a concern they meant the chamber to see, which is the opposite of what a flag
// is for.
func (s *Server) handleFlag(w http.ResponseWriter, r *http.Request) {
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

	flag := r.FormValue("flag")
	if _, ok := flagKinds[flag]; !ok {
		http.Error(w, "unknown flag", http.StatusBadRequest)
		return
	}

	slug := r.FormValue("slug")
	a, ok, err := s.db.ActionBySlug(slug)
	if err != nil || !ok {
		http.NotFound(w, r)
		return
	}

	if _, err := s.db.ToggleFlag(a.ProposalID, member, flag); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/action/"+a.Slug()+"#chamber", http.StatusFound)
}

// handleDraft saves a delegate's private notes on an action.
//
// The notes are private and stay private: they are keyed by the session's own
// identity, so there is no request a delegate can make that returns anyone
// else's. Thinking out loud is not the same as taking a position, and a
// delegate who cannot do the former in confidence will do less of it.
func (s *Server) handleDraft(w http.ResponseWriter, r *http.Request) {
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
	member := strings.TrimSuffix(sessionID, " (demo)")

	a, ok, err := s.db.ActionBySlug(r.FormValue("slug"))
	if err != nil || !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown action"})
		return
	}

	body := r.FormValue("body")
	if len(body) > draftLimit {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
			"error": "these notes are longer than Cella will store",
		})
		return
	}

	if err := s.db.SaveDraft(a.ProposalID, member, body); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "1"})
}

// draftLimit bounds a delegate's notes. Generous enough for real deliberation,
// bounded so an autosaving textarea cannot fill the disk.
const draftLimit = 64 * 1024
