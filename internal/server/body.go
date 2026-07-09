package server

// Constitutional body + member roster shown on the entry splash.
//
// For the demo this is a fixed "Cardano Curia" delegate body with placeholder
// member identities and addresses. In a real deployment the roster would be
// configured (or derived from the on-chain CC credentials that have voted);
// the addresses here are illustrative only.

// Member is one delegate in a constitutional body.
type Member struct {
	Name    string // display identity
	Role    string // portfolio / seat
	Address string // wallet (stake) address — placeholder for the demo
}

// Body is a constitutional committee / delegate consortium.
type Body struct {
	Name    string
	Kind    string // e.g. "Constitutional Committee delegate body"
	Blurb   string
	Members []Member
}

// demoBody is the Cardano Curia roster used for the demo splash.
var demoBody = Body{
	Name:  "Cardano Curia",
	Kind:  "Constitutional Committee member",
	Blurb: "A consortium that deliberates on Cardano governance actions, assesses their constitutionality, and casts a single committee vote with a shared rationale.",
	Members: []Member{
		{Name: "Faustina Vela", Role: "Delegate · Treasury & Withdrawals", Address: "stake1uy9v3k7m2q0f8xw4r6p2n5c8t3l7d1s4h9j0a2b6e5g8c9q7wq2demo01"},
		{Name: "Cassius Aurel", Role: "Delegate · Protocol Parameters", Address: "stake1u84m2n7q9v0k3x8w1r5p6c2t7l4d9s3h8j1a0b5e2g7c6q4wq9demo02"},
		{Name: "Junia Marcia", Role: "Delegate · Constitution & Precedent", Address: "stake1uxk9m3n2q7v8f0x4w6r1p5c9t2l8d3s7h4j6a1b0e9g5c8q3wq5demo03"},
		{Name: "Titus Varo", Role: "Delegate · Community & Outreach", Address: "stake1u7n4m9q2v6k1x0w8r3p7c5t4l2d6s9h1j8a3b7e0g4c2q9wq6demo04"},
		{Name: "Livia Serena", Role: "Delegate · At-large", Address: "stake1u9q2m7n4v0k8x3w1r6p2c7t5l9d4s8h3j1a6b2e5g0c7q4wq1demo05"},
	},
}
