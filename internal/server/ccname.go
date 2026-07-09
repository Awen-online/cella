package server

import "strings"

// CCMember is a seat on the Constitutional Committee: the on-chain cc_hot
// credential and (where known) the readable org name. The 7 authorized seats
// below come from Koios /committee_info; Koios does not carry names, so fill
// them in as you learn them.
type CCMember struct {
	Credential string
	Name       string
}

var ccCommittee = []CCMember{
	{Credential: "cc_hot1qwz0aw5583t56fvcg96ulqjhjk0xkwsuvs2rmp0xflhkh4g5e22ce", Name: "Cardano Curia"},
	{Credential: "cc_hot1qdjx6xe6e9zk3fpzk6rakmz84n0cf8ckwjvz4e8e5j2tuscr7ckq4", Name: ""},
	{Credential: "cc_hot1qvh20fuwhy2dnz9e6d5wmzysduaunlz5y9n8m6n2xen3pmqqvyw8v", Name: ""},
	{Credential: "cc_hot1qdc65ke6jfq2q25fcn3g89tea30tvrzpptc2tw6g8cdc7pqtmus0y", Name: ""},
	{Credential: "cc_hot1qde96n2yfxvx2pc4xm25va9ssqezh5mxhc2n8rdjyxq8kvgwwujd9", Name: ""},
	{Credential: "cc_hot1qf5tkz6zwcpplq3kgpt2486d8za943vmymqkdjl249qgw3s2y5r9y", Name: ""},
	{Credential: "cc_hot1qfj0jatguuhl0cqrtd96u7asszssa3h6uhq08q0dgqzn5jgjfy0l0", Name: ""},
}

// ccNames is a fallback map for credentials outside the committee roster.
var ccNames = map[string]string{}

// ccNamePrefixes matches by credential prefix.
var ccNamePrefixes = map[string]string{}

// ccMemberName returns the readable name for a cc_hot credential, or "".
func ccMemberName(cred string) string {
	for _, m := range ccCommittee {
		if m.Credential == cred {
			return m.Name
		}
	}
	if n, ok := ccNames[cred]; ok {
		return n
	}
	for pfx, n := range ccNamePrefixes {
		if strings.HasPrefix(cred, pfx) {
			return n
		}
	}
	return ""
}
