package main

// ScopeMapping defines NATS permissions granted by an OIDC scope.
type ScopeMapping struct {
	PubAllow []string
	SubAllow []string
}

// DefaultScopeMappings maps OIDC scopes to NATS pub/sub permissions.
var DefaultScopeMappings = map[string]ScopeMapping{
	"nats:admin": {
		PubAllow: []string{">"},
		SubAllow: []string{">"},
	},
	"nats:publish": {
		PubAllow: []string{"orders.>", "events.>"},
		SubAllow: []string{"_INBOX.>"},
	},
	"nats:subscribe": {
		SubAllow: []string{"orders.>", "events.>", "_INBOX.>"},
	},
}

// ResolvedPermissions holds the merged pub/sub permission lists.
type ResolvedPermissions struct {
	PubAllow []string
	SubAllow []string
}

// ResolvePermissions merges all scope mappings for the given scopes.
func ResolvePermissions(scopes []string) *ResolvedPermissions {
	seen := make(map[string]bool)
	result := &ResolvedPermissions{}

	for _, scope := range scopes {
		mapping, ok := DefaultScopeMappings[scope]
		if !ok {
			continue
		}
		for _, s := range mapping.PubAllow {
			key := "pub:" + s
			if !seen[key] {
				result.PubAllow = append(result.PubAllow, s)
				seen[key] = true
			}
		}
		for _, s := range mapping.SubAllow {
			key := "sub:" + s
			if !seen[key] {
				result.SubAllow = append(result.SubAllow, s)
				seen[key] = true
			}
		}
	}

	return result
}

// HasPermissions returns true if any permissions were resolved.
func (p *ResolvedPermissions) HasPermissions() bool {
	return len(p.PubAllow) > 0 || len(p.SubAllow) > 0
}
