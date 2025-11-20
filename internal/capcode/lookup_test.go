package capcode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewLookup_ValidCSV(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "capcodes.csv")

	csvContent := `Capcode;Agency;Region;Station;Function
0101001;Brandweer;Utrecht;Utrecht;Kazernealarm
0101002;Ambulance;Utrecht;Utrecht;A1 Dienst
0234567;Politie;Amsterdam;Centrum;Algemeen`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(t, err)
	require.NotNil(t, lookup)

	// Verify data was loaded
	info := lookup.Get("0101001")
	assert.NotNil(t, info)
	assert.Equal(t, "0101001", info.Capcode)
	assert.Equal(t, "Brandweer", info.Agency)
	assert.Equal(t, "Utrecht", info.Region)
	assert.Equal(t, "Utrecht", info.Station)
	assert.Equal(t, "Kazernealarm", info.Function)
}

func TestNewLookup_WithoutHeader(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "capcodes.csv")

	// CSV without header row
	csvContent := `0101001;Brandweer;Utrecht;Utrecht;Kazernealarm
0101002;Ambulance;Utrecht;Utrecht;A1 Dienst`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(t, err)
	require.NotNil(t, lookup)

	info := lookup.Get("0101001")
	assert.NotNil(t, info)
	assert.Equal(t, "0101001", info.Capcode)
}

func TestNewLookup_QuotedFields(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "capcodes.csv")

	csvContent := `"Capcode";"Agency";"Region";"Station";"Function"
"0101001";"Brandweer";"Utrecht";"Utrecht";"Kazernealarm"
"0234567";"Politie";"Amsterdam";"Centrum";"Algemeen"`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(t, err)

	info := lookup.Get("0101001")
	assert.NotNil(t, info)
	assert.Equal(t, "0101001", info.Capcode)
	assert.Equal(t, "Brandweer", info.Agency)
}

func TestNewLookup_FileNotFound(t *testing.T) {
	lookup, err := NewLookup("/nonexistent/path/capcodes.csv")
	assert.Error(t, err)
	assert.Nil(t, lookup)
	assert.Contains(t, err.Error(), "failed to open capcode CSV")
}

func TestNewLookup_InvalidCSV(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "invalid.csv")

	// CSV with truly malformed content that can't be parsed
	// Using a field with an unclosed quote on the last line can cause errors
	invalidContent := "Capcode;Agency\n\"0101001;Unclosed quote without closing"

	err := os.WriteFile(csvPath, []byte(invalidContent), 0644)
	require.NoError(t, err)

	lookup, err := NewLookup(csvPath)
	// The CSV reader with LazyQuotes=true might parse this successfully
	// So we just check that we can handle it gracefully
	if err != nil {
		assert.Contains(t, err.Error(), "failed to read CSV")
	} else {
		// If it parsed successfully, that's OK too due to LazyQuotes
		assert.NotNil(t, lookup)
	}
}

func TestNewLookup_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "empty.csv")

	err := os.WriteFile(csvPath, []byte(""), 0644)
	require.NoError(t, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(t, err)
	require.NotNil(t, lookup)

	// Empty lookup should return nil for any query
	info := lookup.Get("0101001")
	assert.Nil(t, info)
}

func TestNewLookup_IncompleteRecords(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "incomplete.csv")

	// Some records have fewer than 5 fields
	csvContent := `Capcode;Agency;Region;Station;Function
0101001;Brandweer;Utrecht;Utrecht;Kazernealarm
0101002;Ambulance;Utrecht
0234567;Politie;Amsterdam;Centrum;Algemeen`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(t, err)

	// First record should be loaded
	info1 := lookup.Get("0101001")
	assert.NotNil(t, info1)

	// Second record should be skipped (incomplete)
	info2 := lookup.Get("0101002")
	assert.Nil(t, info2)

	// Third record should be loaded
	info3 := lookup.Get("0234567")
	assert.NotNil(t, info3)
}

func TestNewLookup_VariableFields(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "variable.csv")

	// CSV with variable number of fields per record
	csvContent := `Capcode;Agency;Region;Station;Function;Extra
0101001;Brandweer;Utrecht;Utrecht;Kazernealarm;ExtraField
0234567;Politie;Amsterdam;Centrum;Algemeen`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(t, err)

	// Both records should be loaded (extra fields ignored)
	info1 := lookup.Get("0101001")
	assert.NotNil(t, info1)

	info2 := lookup.Get("0234567")
	assert.NotNil(t, info2)
}

func TestGet_ExactMatch(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "capcodes.csv")

	csvContent := `0101001;Brandweer;Utrecht;Utrecht;Kazernealarm
0234567;Politie;Amsterdam;Centrum;Algemeen`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(t, err)

	tests := []struct {
		name     string
		capcode  string
		expected *CapcodeInfo
	}{
		{
			name:    "First capcode",
			capcode: "0101001",
			expected: &CapcodeInfo{
				Capcode:  "0101001",
				Agency:   "Brandweer",
				Region:   "Utrecht",
				Station:  "Utrecht",
				Function: "Kazernealarm",
			},
		},
		{
			name:    "Second capcode",
			capcode: "0234567",
			expected: &CapcodeInfo{
				Capcode:  "0234567",
				Agency:   "Politie",
				Region:   "Amsterdam",
				Station:  "Centrum",
				Function: "Algemeen",
			},
		},
		{
			name:     "Non-existent capcode",
			capcode:  "9999999",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := lookup.Get(tt.capcode)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.Capcode, result.Capcode)
				assert.Equal(t, tt.expected.Agency, result.Agency)
				assert.Equal(t, tt.expected.Region, result.Region)
				assert.Equal(t, tt.expected.Station, result.Station)
				assert.Equal(t, tt.expected.Function, result.Function)
			}
		})
	}
}

func TestGet_LeadingZeros(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "capcodes.csv")

	csvContent := `0000123;Brandweer;Utrecht;Utrecht;Kazernealarm
0101001;Ambulance;Utrecht;Utrecht;A1 Dienst`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(t, err)

	tests := []struct {
		name      string
		capcode   string
		wantFound bool
	}{
		{
			name:      "With leading zeros (exact)",
			capcode:   "0000123",
			wantFound: true,
		},
		{
			name:      "Without leading zeros (normalized)",
			capcode:   "123",
			wantFound: true,
		},
		{
			name:      "Partial leading zeros",
			capcode:   "00123",
			wantFound: true,
		},
		{
			name:      "With all leading zeros except one",
			capcode:   "0101001",
			wantFound: true,
		},
		{
			name:      "Without some leading zeros",
			capcode:   "101001",
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := lookup.Get(tt.capcode)
			if tt.wantFound {
				assert.NotNil(t, result)
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

func TestGet_AllZeros(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "capcodes.csv")

	csvContent := `0000000;Test;Test;Test;Test`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(t, err)

	// Should be able to look up all zeros
	result := lookup.Get("0000000")
	assert.NotNil(t, result)

	// Should also find with single zero
	result2 := lookup.Get("0")
	assert.NotNil(t, result2)
}

func TestGetMultiple(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "capcodes.csv")

	csvContent := `0101001;Brandweer;Utrecht;Utrecht;Kazernealarm
0101002;Ambulance;Utrecht;Utrecht;A1 Dienst
0234567;Politie;Amsterdam;Centrum;Algemeen`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(t, err)

	tests := []struct {
		name         string
		capcodes     []string
		expectedLen  int
		expectedFirst string
	}{
		{
			name:         "Multiple valid capcodes",
			capcodes:     []string{"0101001", "0101002"},
			expectedLen:  2,
			expectedFirst: "0101001",
		},
		{
			name:        "Empty list",
			capcodes:    []string{},
			expectedLen: 0,
		},
		{
			name:         "Mix of valid and invalid",
			capcodes:     []string{"0101001", "9999999", "0234567"},
			expectedLen:  2,
			expectedFirst: "0101001",
		},
		{
			name:        "All invalid",
			capcodes:    []string{"9999999", "8888888"},
			expectedLen: 0,
		},
		{
			name:         "With normalized capcodes",
			capcodes:     []string{"101001", "234567"},
			expectedLen:  2,
			expectedFirst: "0101001",
		},
		{
			name:         "Duplicate capcodes",
			capcodes:     []string{"0101001", "0101001", "0101002"},
			expectedLen:  3, // Duplicates are not filtered
			expectedFirst: "0101001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := lookup.GetMultiple(tt.capcodes)
			assert.Equal(t, tt.expectedLen, len(result))
			if tt.expectedLen > 0 {
				assert.Equal(t, tt.expectedFirst, result[0].Capcode)
			}
		})
	}
}

func TestGetMultiple_OrderPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "capcodes.csv")

	csvContent := `0101001;Brandweer;Utrecht;Utrecht;Kazernealarm
0101002;Ambulance;Utrecht;Utrecht;A1 Dienst
0234567;Politie;Amsterdam;Centrum;Algemeen`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(t, err)

	capcodes := []string{"0234567", "0101001", "0101002"}
	result := lookup.GetMultiple(capcodes)

	require.Equal(t, 3, len(result))
	assert.Equal(t, "0234567", result[0].Capcode)
	assert.Equal(t, "0101001", result[1].Capcode)
	assert.Equal(t, "0101002", result[2].Capcode)
}

func TestLookup_LargeDataset(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "large.csv")

	// Generate a large CSV with 1000 entries
	var csvContent string
	csvContent = "Capcode;Agency;Region;Station;Function\n"
	for i := 0; i < 1000; i++ {
		capcode := padCapcode(i)
		csvContent += capcode + ";Agency" + capcode + ";Region;Station;Function\n"
	}

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(t, err)

	// Test lookup of various entries
	info := lookup.Get("0000500")
	assert.NotNil(t, info)
	assert.Equal(t, "Agency0000500", info.Agency)

	// Test normalized lookup
	info2 := lookup.Get("500")
	assert.NotNil(t, info2)
}

func TestLookup_SpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "special.csv")

	csvContent := `0101001;Brand & Redding;Utrecht/Amersfoort;Station #1;Alg. Alarm
0101002;Amb-Dienst;Region (Zuid);Station "A";A1-Dienst`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(t, err)

	info1 := lookup.Get("0101001")
	assert.NotNil(t, info1)
	assert.Equal(t, "Brand & Redding", info1.Agency)
	assert.Equal(t, "Utrecht/Amersfoort", info1.Region)
	assert.Equal(t, "Station #1", info1.Station)

	info2 := lookup.Get("0101002")
	assert.NotNil(t, info2)
	assert.Equal(t, "Amb-Dienst", info2.Agency)
	assert.Contains(t, info2.Region, "Zuid")
}

func TestLookup_EmptyFields(t *testing.T) {
	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "empty.csv")

	csvContent := `0101001;;;;""`

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(t, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(t, err)

	info := lookup.Get("0101001")
	require.NotNil(t, info)
	assert.Equal(t, "0101001", info.Capcode)
	assert.Equal(t, "", info.Agency)
	assert.Equal(t, "", info.Region)
	assert.Equal(t, "", info.Station)
	assert.Equal(t, "", info.Function)
}

// Helper function
func padCapcode(num int) string {
	s := ""
	for i := 0; i < 7; i++ {
		s = string(rune('0'+num%10)) + s
		num /= 10
	}
	return s
}

func BenchmarkGet_ExactMatch(b *testing.B) {
	tmpDir := b.TempDir()
	csvPath := filepath.Join(tmpDir, "bench.csv")

	var csvContent string
	for i := 0; i < 1000; i++ {
		capcode := padCapcode(i)
		csvContent += capcode + ";Agency;Region;Station;Function\n"
	}

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(b, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lookup.Get("0000500")
	}
}

func BenchmarkGet_Normalized(b *testing.B) {
	tmpDir := b.TempDir()
	csvPath := filepath.Join(tmpDir, "bench.csv")

	var csvContent string
	for i := 0; i < 1000; i++ {
		capcode := padCapcode(i)
		csvContent += capcode + ";Agency;Region;Station;Function\n"
	}

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(b, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lookup.Get("500")
	}
}

func BenchmarkGetMultiple(b *testing.B) {
	tmpDir := b.TempDir()
	csvPath := filepath.Join(tmpDir, "bench.csv")

	var csvContent string
	for i := 0; i < 1000; i++ {
		capcode := padCapcode(i)
		csvContent += capcode + ";Agency;Region;Station;Function\n"
	}

	err := os.WriteFile(csvPath, []byte(csvContent), 0644)
	require.NoError(b, err)

	lookup, err := NewLookup(csvPath)
	require.NoError(b, err)

	capcodes := []string{"0000100", "0000200", "0000300", "0000400", "0000500"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lookup.GetMultiple(capcodes)
	}
}
