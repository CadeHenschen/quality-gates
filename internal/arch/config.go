package arch

import (
	"encoding/json"
	"fmt"

	"git.roost-r.com/cadeh/quality-gates/internal/safefile"
)

// DefaultRulesFile is the conventional, committable rules file arch-metric
// looks for in --dir when no explicit --rules path is given — same
// pattern as crap-metric's .crap-metric-exclude.
const DefaultRulesFile = ".arch-metric-rules.json"

// config is the on-disk shape of a rules file.
type config struct {
	Rules      []Rule      `json:"rules"`
	Exceptions []Exception `json:"exceptions"`
}

// LoadRules reads and validates a rules file:
//
//	{
//	  "rules": [
//	    {"name": "...", "from": "...", "deny": ["...", "..."]}
//	  ],
//	  "exceptions": [
//	    {"rule": "...", "from": "...", "to": "...", "reason": "..."}
//	  ]
//	}
//
// exceptions is optional; a file with none returns a nil slice.
func LoadRules(path string) (rules []Rule, exceptions []Exception, err error) {
	data, err := safefile.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var cfg config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(cfg.Rules) == 0 {
		return nil, nil, fmt.Errorf("%s: no rules declared", path)
	}
	// Field-level validation (name/from/deny presence, pattern syntax) is
	// Compile's job, not this function's — a rules file is always fed
	// through both in sequence, and duplicating that check here would just
	// be two copies of the same validation to keep in sync.
	return cfg.Rules, cfg.Exceptions, nil
}
