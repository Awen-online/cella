// Package network names the Cardano network a Cella instance is pointed at, and
// derives everything that follows from it.
//
// This matters more than a base URL. Cella hardcoded mainnet in three places —
// the Koios endpoint and, quietly, every explorer link — so an instance
// deliberately pointed at a testnet would still have sent its delegates to
// mainnet to check an action that does not exist there. A committee practising
// on a testnet needs the whole tool to be on that testnet, not most of it.
//
// Note on SanchoNet: it was the pre-Chang governance testnet and Koios does not
// serve it. Governance is live on Preprod and Preview, which Koios does serve,
// so those are the networks Cella supports.
package network

import (
	"fmt"
	"strings"
)

// Network is a Cardano network Cella can run against.
type Network string

const (
	Mainnet Network = "mainnet"
	Preprod Network = "preprod"
	Preview Network = "preview"
)

// Parse reads a network name. An unrecognised name is an error rather than a
// silent fall back to mainnet: an operator who meant to practise on a testnet
// and quietly ended up on mainnet would be voting for real.
func Parse(s string) (Network, error) {
	switch Network(strings.ToLower(strings.TrimSpace(s))) {
	case "", Mainnet:
		return Mainnet, nil
	case Preprod:
		return Preprod, nil
	case Preview:
		return Preview, nil
	case "sanchonet", "sancho":
		return "", fmt.Errorf("SanchoNet is not supported: it was the pre-Chang governance testnet and Koios does not serve it. " +
			"Governance is live on preprod and preview — use CELLA_NETWORK=preprod")
	default:
		return "", fmt.Errorf("unknown network %q: use mainnet, preprod or preview", s)
	}
}

// IsTestnet reports whether votes cast here are practice rather than governance.
func (n Network) IsTestnet() bool { return n != Mainnet }

// KoiosURL is the Koios instance serving this network.
func (n Network) KoiosURL() string {
	switch n {
	case Preprod:
		return "https://preprod.koios.rest/api/v1"
	case Preview:
		return "https://preview.koios.rest/api/v1"
	default:
		return "https://api.koios.rest/api/v1"
	}
}

// ExplorerAction links to a governance action on a block explorer for this
// network. govID is the explorer's form: the tx hash followed by the cert index
// as two hex digits.
func (n Network) ExplorerAction(govID string) string {
	return n.explorerBase() + "/governances/" + govID
}

// ExplorerTx links to a transaction.
func (n Network) ExplorerTx(txHash string) string {
	return n.explorerBase() + "/transactions/" + txHash
}

func (n Network) explorerBase() string {
	switch n {
	case Preprod:
		return "https://preprod.adastat.net"
	case Preview:
		return "https://preview.adastat.net"
	default:
		return "https://adastat.net"
	}
}

// AddressPrefix is the bech32 prefix a payment address carries on this network.
// Cella uses it to catch a roster written for the wrong network — a mainnet
// address on a testnet instance will never match a signature, and finding that
// out at sign-in is too late.
func (n Network) AddressPrefix() string {
	if n.IsTestnet() {
		return "addr_test"
	}
	return "addr"
}

// Label is the network's name, for display.
func (n Network) Label() string { return string(n) }
