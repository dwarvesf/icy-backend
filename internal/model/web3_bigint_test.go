package model

import (
	"testing"
)

func TestWeb3BigInt_ToFloat(t *testing.T) {
	tests := []struct {
		name     string
		input    Web3BigInt
		expected float64
	}{
		{
			name: "simple number",
			input: Web3BigInt{
				Value:   "1000000",
				Decimal: 6,
			},
			expected: 1.0,
		},
		{
			name: "zero value",
			input: Web3BigInt{
				Value:   "0",
				Decimal: 18,
			},
			expected: 0.0,
		},
		{
			name: "large number",
			input: Web3BigInt{
				Value:   "1234567890000000000",
				Decimal: 18,
			},
			expected: 1.23456789,
		},
		{
			name: "small decimal",
			input: Web3BigInt{
				Value:   "123456",
				Decimal: 3,
			},
			expected: 123.456,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.input.ToFloat()
			if result != tt.expected {
				t.Errorf("ToFloat() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestWeb3BigInt_Add(t *testing.T) {
	tests := []struct {
		name     string
		a        Web3BigInt
		b        Web3BigInt
		expected *Web3BigInt
	}{
		{
			name: "simple addition",
			a: Web3BigInt{
				Value:   "1000000",
				Decimal: 6,
			},
			b: Web3BigInt{
				Value:   "2000000",
				Decimal: 6,
			},
			expected: &Web3BigInt{
				Value:   "3000000",
				Decimal: 6,
			},
		},
		{
			name: "add zero",
			a: Web3BigInt{
				Value:   "1000000",
				Decimal: 18,
			},
			b: Web3BigInt{
				Value:   "0",
				Decimal: 18,
			},
			expected: &Web3BigInt{
				Value:   "1000000",
				Decimal: 18,
			},
		},
		{
			name: "different decimals",
			a: Web3BigInt{
				Value:   "1000000",
				Decimal: 6,
			},
			b: Web3BigInt{
				Value:   "1000000",
				Decimal: 18,
			},
			expected: nil,
		},
		{
			name: "large numbers",
			a: Web3BigInt{
				Value:   "999999999999999999999999",
				Decimal: 18,
			},
			b: Web3BigInt{
				Value:   "1",
				Decimal: 18,
			},
			expected: &Web3BigInt{
				Value:   "1000000000000000000000000",
				Decimal: 18,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.a.Add(&tt.b)
			if result == nil && tt.expected == nil {
				return
			}
			if result == nil || tt.expected == nil {
				t.Errorf("Add() = %v, want %v", result, tt.expected)
				return
			}
			if result.Value != tt.expected.Value || result.Decimal != tt.expected.Decimal {
				t.Errorf("Add() = {%v, %v}, want {%v, %v}",
					result.Value, result.Decimal,
					tt.expected.Value, tt.expected.Decimal)
			}
		})
	}
}

func TestWeb3BigInt_Sub(t *testing.T) {
	tests := []struct {
		name     string
		a        Web3BigInt
		b        Web3BigInt
		expected *Web3BigInt
	}{
		{
			name: "simple subtraction",
			a: Web3BigInt{
				Value:   "3000000",
				Decimal: 6,
			},
			b: Web3BigInt{
				Value:   "1000000",
				Decimal: 6,
			},
			expected: &Web3BigInt{
				Value:   "2000000",
				Decimal: 6,
			},
		},
		{
			name: "subtract to zero",
			a: Web3BigInt{
				Value:   "1000000",
				Decimal: 18,
			},
			b: Web3BigInt{
				Value:   "1000000",
				Decimal: 18,
			},
			expected: &Web3BigInt{
				Value:   "0",
				Decimal: 18,
			},
		},
		{
			name: "different decimals",
			a: Web3BigInt{
				Value:   "3000000",
				Decimal: 6,
			},
			b: Web3BigInt{
				Value:   "1000000",
				Decimal: 18,
			},
			expected: nil,
		},
		{
			name: "negative result",
			a: Web3BigInt{
				Value:   "1000000",
				Decimal: 6,
			},
			b: Web3BigInt{
				Value:   "2000000",
				Decimal: 6,
			},
			expected: &Web3BigInt{
				Value:   "-1000000",
				Decimal: 6,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.a.Sub(&tt.b)
			if result == nil && tt.expected == nil {
				return
			}
			if result == nil || tt.expected == nil {
				t.Errorf("Sub() = %v, want %v", result, tt.expected)
				return
			}
			if result.Value != tt.expected.Value || result.Decimal != tt.expected.Decimal {
				t.Errorf("Sub() = {%v, %v}, want {%v, %v}",
					result.Value, result.Decimal,
					tt.expected.Value, tt.expected.Decimal)
			}
		})
	}
}
