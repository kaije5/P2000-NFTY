package filter

import (
	"github.com/rs/zerolog"
)

// CapcodeFilter filters messages based on exact capcode matches
type CapcodeFilter struct {
	forwardAll  bool
	allowedCaps map[string]struct{}
	logger      zerolog.Logger
}

// NewCapcodeFilter creates a new capcode filter
func NewCapcodeFilter(forwardAll bool, capcodes []string, logger zerolog.Logger) *CapcodeFilter {
	allowedCaps := make(map[string]struct{}, len(capcodes))
	for _, capcode := range capcodes {
		allowedCaps[capcode] = struct{}{}
	}

	if forwardAll {
		logger.Info().Msg("capcode filter initialized with forward_all=true (all messages will be forwarded)")
	} else {
		logger.Info().
			Int("count", len(capcodes)).
			Msg("capcode filter initialized with specific capcodes")
	}

	return &CapcodeFilter{
		forwardAll:  forwardAll,
		allowedCaps: allowedCaps,
		logger:      logger,
	}
}

// ShouldForward checks if any capcode in the list matches the filter
func (f *CapcodeFilter) ShouldForward(capcodes []string) bool {
	// If forward_all is enabled, always forward messages
	if f.forwardAll {
		f.logger.Debug().
			Strs("capcodes", capcodes).
			Msg("forwarding message (forward_all enabled)")
		return true
	}

	// Otherwise, check capcode filter
	if len(capcodes) == 0 {
		return false
	}

	for _, capcode := range capcodes {
		if _, exists := f.allowedCaps[capcode]; exists {
			f.logger.Debug().
				Str("matched_capcode", capcode).
				Msg("capcode match found")
			return true
		}
	}

	f.logger.Debug().
		Strs("capcodes", capcodes).
		Msg("no capcode match")
	return false
}

// Count returns the number of configured capcodes
func (f *CapcodeFilter) Count() int {
	return len(f.allowedCaps)
}
