package chute

import (
	"sort"
	"strings"
)

type EvidenceTier string

const (
	TierPrimary    EvidenceTier = "primary"
	TierSecondary  EvidenceTier = "secondary"
	TierContextual EvidenceTier = "contextual"
)

type EvidenceIdentifier struct {
	Value string       `json:"value"`
	Tier  EvidenceTier `json:"tier"`
	Kind  string       `json:"kind"`
}

func evidenceTierRank(tier EvidenceTier) int {
	switch tier {
	case TierPrimary:
		return 0
	case TierSecondary:
		return 1
	default:
		return 2
	}
}

func strongerTier(a, b EvidenceTier) EvidenceTier {
	if evidenceTierRank(a) <= evidenceTierRank(b) {
		return a
	}
	return b
}

func normalizeEvidenceIdentifiers(values []EvidenceIdentifier) []EvidenceIdentifier {
	seen := make(map[string]EvidenceIdentifier)
	for _, identifier := range values {
		identifier.Value = strings.TrimSpace(identifier.Value)
		if identifier.Value == "" || len(identifier.Value) < 4 {
			continue
		}
		if existing, ok := seen[identifier.Value]; ok {
			if evidenceTierRank(identifier.Tier) < evidenceTierRank(existing.Tier) {
				seen[identifier.Value] = identifier
			}
			continue
		}
		seen[identifier.Value] = identifier
	}

	out := make([]EvidenceIdentifier, 0, len(seen))
	for _, identifier := range seen {
		out = append(out, identifier)
	}
	sort.Slice(out, func(i, j int) bool {
		if evidenceTierRank(out[i].Tier) != evidenceTierRank(out[j].Tier) {
			return evidenceTierRank(out[i].Tier) < evidenceTierRank(out[j].Tier)
		}
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Value < out[j].Value
	})
	return out
}

func identifierValues(identifiers []EvidenceIdentifier) []string {
	out := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		out = append(out, identifier.Value)
	}
	return out
}

func addResourceIdentifiers(out []EvidenceIdentifier, resource *Resource, tier EvidenceTier, kind string) []EvidenceIdentifier {
	if resource == nil {
		return out
	}
	if resource.Name != "" {
		out = append(out, EvidenceIdentifier{Value: resource.Name, Tier: tier, Kind: kind + ".name"})
	}
	if resource.UID != "" {
		out = append(out, EvidenceIdentifier{Value: resource.UID, Tier: tier, Kind: kind + ".uid"})
	}
	return out
}

func addResourceSliceIdentifiers(out []EvidenceIdentifier, resources []Resource, tier EvidenceTier, kind string) []EvidenceIdentifier {
	for i := range resources {
		out = addResourceIdentifiers(out, &resources[i], tier, kind)
	}
	return out
}
