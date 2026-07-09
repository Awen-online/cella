package server

import (
	"net/http"
	"strings"
)

// handleCastVote records the signed-in delegate's internal position + rationale
// on an action (stored in the shared instance so co-members see it).
func (s *Server) handleCastVote(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	member, ok := s.member(r)
	if !ok {
		http.Redirect(w, r, "/enter", http.StatusFound)
		return
	}
	member = strings.TrimSuffix(member, " (demo)")

	slug := r.FormValue("slug")
	vote := r.FormValue("vote")
	rationale := strings.TrimSpace(r.FormValue("rationale"))
	if vote != "Yes" && vote != "No" && vote != "Abstain" {
		http.Redirect(w, r, "/action/"+slug, http.StatusFound)
		return
	}

	a, ok, err := s.db.ActionBySlug(slug)
	if err != nil || !ok {
		http.NotFound(w, r)
		return
	}
	if err := s.db.UpsertMemberVote(a.ProposalID, member, vote, rationale); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/action/"+a.Slug()+"#your-position", http.StatusFound)
}
