package protocol

import "strings"

// IsCodeProposalKind reports whether kind is a code.proposal family variant.
func IsCodeProposalKind(kind string) bool {
	switch MutationKind(strings.TrimSpace(kind)) {
	case MutationCodeProposal, MutationCodeProposalIsolated, MutationCodeProposalRoot:
		return true
	default:
		return false
	}
}

// NormalizeCodeProposalKind maps bare alias to isolated; other kinds pass through unchanged.
func NormalizeCodeProposalKind(kind MutationKind) MutationKind {
	switch kind {
	case MutationCodeProposal, MutationCodeProposalIsolated:
		return MutationCodeProposalIsolated
	default:
		return kind
	}
}

// CodeProposalKindsMatch reports whether a subscriber rule kind matches an event kind.
// Isolated family (alias + isolated) matches only itself; root matches only root.
func CodeProposalKindsMatch(subscriberKind, eventKind string) bool {
	sub := codeProposalMatchFamily(subscriberKind)
	if sub == "" {
		return false
	}
	return sub == codeProposalMatchFamily(eventKind)
}

func codeProposalMatchFamily(kind string) string {
	switch MutationKind(strings.TrimSpace(kind)) {
	case MutationCodeProposal, MutationCodeProposalIsolated:
		return string(MutationCodeProposalIsolated)
	case MutationCodeProposalRoot:
		return string(MutationCodeProposalRoot)
	default:
		return ""
	}
}
