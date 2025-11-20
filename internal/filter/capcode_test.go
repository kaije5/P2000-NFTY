package filter

import (
	"bytes"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func getTestLogger() zerolog.Logger {
	var buf bytes.Buffer
	return zerolog.New(&buf).With().Timestamp().Logger()
}

func TestNewCapcodeFilter(t *testing.T) {
	logger := getTestLogger()

	tests := []struct {
		name       string
		forwardAll bool
		capcodes   []string
		wantCount  int
	}{
		{
			name:       "ForwardAll enabled with empty capcodes",
			forwardAll: true,
			capcodes:   []string{},
			wantCount:  0,
		},
		{
			name:       "ForwardAll enabled with capcodes",
			forwardAll: true,
			capcodes:   []string{"0101001", "0101002"},
			wantCount:  2,
		},
		{
			name:       "ForwardAll disabled with single capcode",
			forwardAll: false,
			capcodes:   []string{"0101001"},
			wantCount:  1,
		},
		{
			name:       "ForwardAll disabled with multiple capcodes",
			forwardAll: false,
			capcodes:   []string{"0101001", "0101002", "0101003"},
			wantCount:  3,
		},
		{
			name:       "Empty capcodes",
			forwardAll: false,
			capcodes:   []string{},
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewCapcodeFilter(tt.forwardAll, tt.capcodes, logger)
			assert.NotNil(t, filter)
			assert.Equal(t, tt.forwardAll, filter.forwardAll)
			assert.Equal(t, tt.wantCount, filter.Count())
		})
	}
}

func TestShouldForward_ForwardAllEnabled(t *testing.T) {
	logger := getTestLogger()
	filter := NewCapcodeFilter(true, []string{"0101001"}, logger)

	tests := []struct {
		name     string
		capcodes []string
		want     bool
	}{
		{
			name:     "Empty capcodes",
			capcodes: []string{},
			want:     true,
		},
		{
			name:     "Single capcode",
			capcodes: []string{"0101001"},
			want:     true,
		},
		{
			name:     "Multiple capcodes",
			capcodes: []string{"0101001", "0101002"},
			want:     true,
		},
		{
			name:     "Unmatched capcodes",
			capcodes: []string{"9999999"},
			want:     true,
		},
		{
			name:     "Mixed matched and unmatched",
			capcodes: []string{"0101001", "9999999"},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.ShouldForward(tt.capcodes)
			assert.Equal(t, tt.want, result, "ForwardAll should always return true")
		})
	}
}

func TestShouldForward_ForwardAllDisabled(t *testing.T) {
	logger := getTestLogger()
	allowedCapcodes := []string{"0101001", "0101002", "0101003"}
	filter := NewCapcodeFilter(false, allowedCapcodes, logger)

	tests := []struct {
		name     string
		capcodes []string
		want     bool
	}{
		{
			name:     "Empty capcodes list",
			capcodes: []string{},
			want:     false,
		},
		{
			name:     "Single matching capcode",
			capcodes: []string{"0101001"},
			want:     true,
		},
		{
			name:     "Single non-matching capcode",
			capcodes: []string{"9999999"},
			want:     false,
		},
		{
			name:     "Multiple matching capcodes",
			capcodes: []string{"0101001", "0101002"},
			want:     true,
		},
		{
			name:     "Multiple non-matching capcodes",
			capcodes: []string{"9999999", "8888888"},
			want:     false,
		},
		{
			name:     "Mixed with one match",
			capcodes: []string{"9999999", "0101001", "8888888"},
			want:     true,
		},
		{
			name:     "First capcode matches",
			capcodes: []string{"0101001", "9999999"},
			want:     true,
		},
		{
			name:     "Last capcode matches",
			capcodes: []string{"9999999", "0101003"},
			want:     true,
		},
		{
			name:     "All allowed capcodes",
			capcodes: []string{"0101001", "0101002", "0101003"},
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.ShouldForward(tt.capcodes)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestShouldForward_EdgeCases(t *testing.T) {
	logger := getTestLogger()

	t.Run("Empty filter with empty capcodes", func(t *testing.T) {
		filter := NewCapcodeFilter(false, []string{}, logger)
		result := filter.ShouldForward([]string{})
		assert.False(t, result)
	})

	t.Run("Empty filter with non-empty capcodes", func(t *testing.T) {
		filter := NewCapcodeFilter(false, []string{}, logger)
		result := filter.ShouldForward([]string{"0101001"})
		assert.False(t, result)
	})

	t.Run("Special characters in capcodes", func(t *testing.T) {
		filter := NewCapcodeFilter(false, []string{"ABC-123", "DEF_456"}, logger)
		assert.True(t, filter.ShouldForward([]string{"ABC-123"}))
		assert.True(t, filter.ShouldForward([]string{"DEF_456"}))
		assert.False(t, filter.ShouldForward([]string{"ABC123"}))
	})

	t.Run("Case sensitivity", func(t *testing.T) {
		filter := NewCapcodeFilter(false, []string{"abc123"}, logger)
		assert.True(t, filter.ShouldForward([]string{"abc123"}))
		assert.False(t, filter.ShouldForward([]string{"ABC123"}))
		assert.False(t, filter.ShouldForward([]string{"Abc123"}))
	})

	t.Run("Leading zeros", func(t *testing.T) {
		filter := NewCapcodeFilter(false, []string{"0101001"}, logger)
		assert.True(t, filter.ShouldForward([]string{"0101001"}))
		assert.False(t, filter.ShouldForward([]string{"101001"}))
	})

	t.Run("Whitespace in capcodes", func(t *testing.T) {
		filter := NewCapcodeFilter(false, []string{"0101001", " 0101002"}, logger)
		assert.True(t, filter.ShouldForward([]string{"0101001"}))
		assert.True(t, filter.ShouldForward([]string{" 0101002"}))
		assert.False(t, filter.ShouldForward([]string{"0101002"}))
	})

	t.Run("Duplicate capcodes in allowed list", func(t *testing.T) {
		filter := NewCapcodeFilter(false, []string{"0101001", "0101001", "0101002"}, logger)
		// Map deduplicates, so count should be 2
		assert.Equal(t, 2, filter.Count())
		assert.True(t, filter.ShouldForward([]string{"0101001"}))
		assert.True(t, filter.ShouldForward([]string{"0101002"}))
	})
}

func TestCount(t *testing.T) {
	logger := getTestLogger()

	tests := []struct {
		name      string
		capcodes  []string
		wantCount int
	}{
		{
			name:      "No capcodes",
			capcodes:  []string{},
			wantCount: 0,
		},
		{
			name:      "Single capcode",
			capcodes:  []string{"0101001"},
			wantCount: 1,
		},
		{
			name:      "Multiple capcodes",
			capcodes:  []string{"0101001", "0101002", "0101003"},
			wantCount: 3,
		},
		{
			name:      "Duplicates are deduplicated",
			capcodes:  []string{"0101001", "0101001", "0101002"},
			wantCount: 2,
		},
		{
			name:      "Large number of capcodes",
			capcodes:  generateCapcodes(100),
			wantCount: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewCapcodeFilter(false, tt.capcodes, logger)
			assert.Equal(t, tt.wantCount, filter.Count())
		})
	}
}

func TestPerformance(t *testing.T) {
	logger := getTestLogger()

	// Test with large number of capcodes
	largeCapcodeList := generateCapcodes(10000)
	filter := NewCapcodeFilter(false, largeCapcodeList, logger)

	assert.Equal(t, 10000, filter.Count())

	// Test lookup performance (should be O(1) with map)
	testCases := [][]string{
		{"0000000"},  // First
		{"0005000"},  // Middle
		{"0009999"},  // Last
		{"9999999"},  // Not found
	}

	for _, capcodes := range testCases {
		_ = filter.ShouldForward(capcodes)
	}
}

func TestConcurrentAccess(t *testing.T) {
	logger := getTestLogger()
	filter := NewCapcodeFilter(false, []string{"0101001", "0101002"}, logger)

	// Test concurrent reads (should be safe since no writes)
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				filter.ShouldForward([]string{"0101001"})
				filter.Count()
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

// Helper function to generate test capcodes
func generateCapcodes(count int) []string {
	capcodes := make([]string, count)
	for i := 0; i < count; i++ {
		// Generate capcodes like "0000000", "0000001", ..., "0009999"
		capcodes[i] = padCapcode(i)
	}
	return capcodes
}

func padCapcode(num int) string {
	s := ""
	for i := 0; i < 7; i++ {
		s = string(rune('0'+num%10)) + s
		num /= 10
	}
	return s
}

func BenchmarkShouldForward_ForwardAll(b *testing.B) {
	logger := getTestLogger()
	filter := NewCapcodeFilter(true, []string{}, logger)
	capcodes := []string{"0101001", "0101002", "0101003"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.ShouldForward(capcodes)
	}
}

func BenchmarkShouldForward_SmallFilter(b *testing.B) {
	logger := getTestLogger()
	filter := NewCapcodeFilter(false, []string{"0101001", "0101002", "0101003"}, logger)
	capcodes := []string{"0101001"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.ShouldForward(capcodes)
	}
}

func BenchmarkShouldForward_LargeFilter(b *testing.B) {
	logger := getTestLogger()
	largeList := generateCapcodes(10000)
	filter := NewCapcodeFilter(false, largeList, logger)
	capcodes := []string{"0005000"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.ShouldForward(capcodes)
	}
}

func BenchmarkShouldForward_NoMatch(b *testing.B) {
	logger := getTestLogger()
	filter := NewCapcodeFilter(false, []string{"0101001", "0101002", "0101003"}, logger)
	capcodes := []string{"9999999"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		filter.ShouldForward(capcodes)
	}
}
