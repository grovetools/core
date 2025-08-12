package models

import (
	"database/sql/driver"
	"fmt"
	"testing"
	"time"
)

type TestStruct struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestJSONField_Value(t *testing.T) {
	tests := []struct {
		name     string
		field    JSONField[TestStruct]
		expected string
	}{
		{
			name: "marshal simple struct",
			field: JSONField[TestStruct]{
				Data: TestStruct{Name: "test", Value: 42},
			},
			expected: `{"name":"test","value":42}`,
		},
		{
			name: "marshal empty struct",
			field: JSONField[TestStruct]{
				Data: TestStruct{},
			},
			expected: `{"name":"","value":0}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := tt.field.Value()
			if err != nil {
				t.Errorf("JSONField.Value() error = %v", err)
				return
			}

			bytes, ok := value.([]byte)
			if !ok {
				t.Errorf("JSONField.Value() returned type %T, expected []byte", value)
				return
			}

			if string(bytes) != tt.expected {
				t.Errorf("JSONField.Value() = %s, want %s", string(bytes), tt.expected)
			}
		})
	}
}

func TestJSONField_Value_MarshalError(t *testing.T) {
	// Test marshal error with invalid data
	field := JSONField[func()]{
		Data: func() {}, // functions can't be marshaled
	}

	_, err := field.Value()
	if err == nil {
		t.Error("JSONField.Value() expected error for unmarshalable data, got nil")
	}
}

func TestJSONField_Scan(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected TestStruct
		wantErr  bool
	}{
		{
			name:     "scan byte array",
			input:    []byte(`{"name":"test","value":42}`),
			expected: TestStruct{Name: "test", Value: 42},
			wantErr:  false,
		},
		{
			name:     "scan string",
			input:    `{"name":"test","value":42}`,
			expected: TestStruct{Name: "test", Value: 42},
			wantErr:  false,
		},
		{
			name:     "scan nil",
			input:    nil,
			expected: TestStruct{},
			wantErr:  false,
		},
		{
			name:     "scan empty byte array",
			input:    []byte(`{}`),
			expected: TestStruct{},
			wantErr:  false,
		},
		{
			name:    "scan invalid json",
			input:   []byte(`{"name":"test","value":}`),
			wantErr: true,
		},
		{
			name:    "scan invalid type",
			input:   123,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var field JSONField[TestStruct]
			err := field.Scan(tt.input)

			if (err != nil) != tt.wantErr {
				t.Errorf("JSONField.Scan() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && field.Data != tt.expected {
				t.Errorf("JSONField.Scan() = %+v, want %+v", field.Data, tt.expected)
			}
		})
	}
}

func TestJSONField_Scan_ComplexTypes(t *testing.T) {
	t.Run("scan map", func(t *testing.T) {
		var field JSONField[map[string]interface{}]
		input := []byte(`{"key":"value","number":42}`)

		err := field.Scan(input)
		if err != nil {
			t.Errorf("JSONField.Scan() error = %v", err)
			return
		}

		if field.Data["key"] != "value" {
			t.Errorf("Expected key='value', got %v", field.Data["key"])
		}
		if field.Data["number"] != float64(42) {
			t.Errorf("Expected number=42, got %v", field.Data["number"])
		}
	})

	t.Run("scan slice", func(t *testing.T) {
		var field JSONField[[]string]
		input := []byte(`["a","b","c"]`)

		err := field.Scan(input)
		if err != nil {
			t.Errorf("JSONField.Scan() error = %v", err)
			return
		}

		expected := []string{"a", "b", "c"}
		if len(field.Data) != len(expected) {
			t.Errorf("Expected length %d, got %d", len(expected), len(field.Data))
			return
		}

		for i, v := range expected {
			if field.Data[i] != v {
				t.Errorf("Expected field.Data[%d]=%s, got %s", i, v, field.Data[i])
			}
		}
	})
}

func TestJSONField_DatabaseIntegration(t *testing.T) {
	// Test that JSONField implements driver.Valuer and sql.Scanner
	var field JSONField[TestStruct]

	// Test Value() returns driver.Value
	_, ok := interface{}(field).(driver.Valuer)
	if !ok {
		t.Error("JSONField should implement driver.Valuer")
	}

	// Test Scan() takes interface{}
	var ptr interface{} = &field
	if _, ok := ptr.(interface{ Scan(interface{}) error }); !ok {
		t.Error("JSONField should implement sql.Scanner")
	}
}

func TestResult(t *testing.T) {
	t.Run("success result", func(t *testing.T) {
		result := Result[string]{
			Data: "test data",
			Ok:   true,
		}

		if result.Data != "test data" {
			t.Errorf("Expected data='test data', got %s", result.Data)
		}
		if !result.Ok {
			t.Error("Expected Ok=true")
		}
		if result.Error != "" {
			t.Errorf("Expected empty error, got %s", result.Error)
		}
	})

	t.Run("error result", func(t *testing.T) {
		result := Result[string]{
			Error: "something went wrong",
			Ok:    false,
		}

		if result.Data != "" {
			t.Errorf("Expected empty data, got %s", result.Data)
		}
		if result.Ok {
			t.Error("Expected Ok=false")
		}
		if result.Error != "something went wrong" {
			t.Errorf("Expected error='something went wrong', got %s", result.Error)
		}
	})
}

func TestPage(t *testing.T) {
	items := []string{"a", "b", "c"}
	page := Page[string]{
		Items:    items,
		Total:    10,
		Page:     2,
		PageSize: 3,
		HasNext:  true,
		HasPrev:  true,
	}

	if len(page.Items) != 3 {
		t.Errorf("Expected 3 items, got %d", len(page.Items))
	}
	if page.Total != 10 {
		t.Errorf("Expected total=10, got %d", page.Total)
	}
	if page.Page != 2 {
		t.Errorf("Expected page=2, got %d", page.Page)
	}
	if !page.HasNext {
		t.Error("Expected HasNext=true")
	}
	if !page.HasPrev {
		t.Error("Expected HasPrev=true")
	}
}

func TestFilter(t *testing.T) {
	now := time.Now()
	filter := Filter{
		StartTime: &now,
		EndTime:   &now,
		Status:    []string{"active", "pending"},
		Tags:      []string{"test", "dev"},
		Metadata:  map[string]string{"key": "value"},
		OrderBy:   "created_at",
		Limit:     10,
		Offset:    20,
	}

	if filter.StartTime == nil {
		t.Error("Expected StartTime to be set")
	}
	if len(filter.Status) != 2 {
		t.Errorf("Expected 2 status values, got %d", len(filter.Status))
	}
	if filter.Metadata["key"] != "value" {
		t.Errorf("Expected metadata key='value', got %s", filter.Metadata["key"])
	}
	if filter.Limit != 10 {
		t.Errorf("Expected limit=10, got %d", filter.Limit)
	}
}

func TestFilter_Validate(t *testing.T) {
	tests := []struct {
		name    string
		filter  Filter
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid filter",
			filter: Filter{
				Status:   []string{"active"},
				Tags:     []string{"test"},
				Metadata: map[string]string{"key": "value"},
				Limit:    10,
				Offset:   0,
			},
			wantErr: false,
		},
		{
			name: "too many status filters",
			filter: Filter{
				Status: make([]string, MaxFilterStatusCount+1),
			},
			wantErr: true,
			errMsg:  "too many status filters",
		},
		{
			name: "too many tag filters",
			filter: Filter{
				Tags: make([]string, MaxFilterTagsCount+1),
			},
			wantErr: true,
			errMsg:  "too many tag filters",
		},
		{
			name: "too many metadata filters",
			filter: Filter{
				Metadata: func() map[string]string {
					m := make(map[string]string)
					for i := 0; i < MaxFilterMetadataCount+1; i++ {
						m[fmt.Sprintf("key%d", i)] = "value"
					}
					return m
				}(),
			},
			wantErr: true,
			errMsg:  "too many metadata filters",
		},
		{
			name: "limit too large",
			filter: Filter{
				Limit: MaxFilterLimit + 1,
			},
			wantErr: true,
			errMsg:  "limit too large",
		},
		{
			name: "negative limit",
			filter: Filter{
				Limit: -1,
			},
			wantErr: true,
			errMsg:  "limit cannot be negative",
		},
		{
			name: "negative offset",
			filter: Filter{
				Offset: -1,
			},
			wantErr: true,
			errMsg:  "offset cannot be negative",
		},
		{
			name: "start time after end time",
			filter: Filter{
				StartTime: &[]time.Time{time.Now()}[0],
				EndTime:   &[]time.Time{time.Now().Add(-time.Hour)}[0],
			},
			wantErr: true,
			errMsg:  "start time cannot be after end time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.filter.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Filter.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err != nil && tt.errMsg != "" {
				if !contains(err.Error(), tt.errMsg) {
					t.Errorf("Filter.Validate() error = %v, want error containing %s", err, tt.errMsg)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
			return false
		}())
}

func BenchmarkJSONField_Value(b *testing.B) {
	field := JSONField[TestStruct]{
		Data: TestStruct{Name: "test", Value: 42},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := field.Value()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONField_Scan(b *testing.B) {
	input := []byte(`{"name":"test","value":42}`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var field JSONField[TestStruct]
		err := field.Scan(input)
		if err != nil {
			b.Fatal(err)
		}
	}
}
