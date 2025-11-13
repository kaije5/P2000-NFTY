package capcode

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
)

// CapcodeInfo contains information about a capcode from the CSV
type CapcodeInfo struct {
	Capcode  string
	Agency   string
	Region   string
	Station  string
	Function string
}

// Lookup provides capcode information lookup functionality
type Lookup struct {
	data map[string]CapcodeInfo
}

// NewLookup creates a new capcode lookup from a CSV file
func NewLookup(csvPath string) (*Lookup, error) {
	file, err := os.Open(csvPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open capcode CSV: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = ';'
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1 // Allow variable number of fields

	// Read all records
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	lookup := &Lookup{
		data: make(map[string]CapcodeInfo),
	}

	// Skip header row if it exists
	startIdx := 0
	if len(records) > 0 {
		// Check if first row is a header by looking for "capcode" or "agency"
		firstRow := strings.ToLower(records[0][0])
		if strings.Contains(firstRow, "capcode") || strings.Contains(firstRow, "cap") {
			startIdx = 1
		}
	}

	// Parse records
	for i := startIdx; i < len(records); i++ {
		record := records[i]
		if len(record) < 5 {
			continue // Skip incomplete records
		}

		capcode := strings.Trim(record[0], `"`)
		info := CapcodeInfo{
			Capcode:  capcode,
			Agency:   strings.Trim(record[1], `"`),
			Region:   strings.Trim(record[2], `"`),
			Station:  strings.Trim(record[3], `"`),
			Function: strings.Trim(record[4], `"`),
		}

		// Store with normalized capcode (without leading zeros) as key
		normalizedKey := strings.TrimLeft(capcode, "0")
		if normalizedKey == "" {
			normalizedKey = "0"
		}
		lookup.data[normalizedKey] = info

		// Also store with original capcode for exact matches
		lookup.data[capcode] = info
	}

	return lookup, nil
}

// Get retrieves capcode information, returns nil if not found
// Handles both formats with and without leading zeros
func (l *Lookup) Get(capcode string) *CapcodeInfo {
	// Try exact match first
	if info, ok := l.data[capcode]; ok {
		return &info
	}

	// Try normalized version (without leading zeros)
	normalized := strings.TrimLeft(capcode, "0")
	if normalized == "" {
		normalized = "0"
	}

	if info, ok := l.data[normalized]; ok {
		return &info
	}

	return nil
}

// GetMultiple retrieves information for multiple capcodes
func (l *Lookup) GetMultiple(capcodes []string) []CapcodeInfo {
	result := make([]CapcodeInfo, 0, len(capcodes))

	for _, capcode := range capcodes {
		if info := l.Get(capcode); info != nil {
			result = append(result, *info)
		}
	}

	return result
}
