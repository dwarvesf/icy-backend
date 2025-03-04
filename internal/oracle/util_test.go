package oracle

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/dwarvesf/icy-backend/internal/model"
)

func TestGetConversionRatio(t *testing.T) {
	tests := []struct {
		name          string
		circulatedIcy *model.Web3BigInt
		btcSupply     *model.Web3BigInt
		want          *model.Web3BigInt
		wantErr       error
	}{
		{
			name: "success - normal case",
			circulatedIcy: &model.Web3BigInt{
				Value:   "1000000000000000000", // 1 ICY with 18 decimals
				Decimal: 18,
			},
			btcSupply: &model.Web3BigInt{
				Value:   "20000000000", // 2 BTC with 10 decimals
				Decimal: 10,
			},
			want: &model.Web3BigInt{
				Value:   "50000000", // 0.5 with 8 decimals
				Decimal: 8,
			},
			wantErr: nil,
		},
		{
			name: "error - btc supply is zero",
			circulatedIcy: &model.Web3BigInt{
				Value:   "1000000000000000000",
				Decimal: 18,
			},
			btcSupply: &model.Web3BigInt{
				Value:   "0",
				Decimal: 10,
			},
			want: &model.Web3BigInt{
				Value:   "0", // 0 with 8 decimals
				Decimal: 8,
			},
			wantErr: nil,
		},
		{
			name: "success - large numbers",
			circulatedIcy: &model.Web3BigInt{
				Value:   "100000000000000000000000", // 100,000 ICY with 18 decimals
				Decimal: 18,
			},
			btcSupply: &model.Web3BigInt{
				Value:   "100000000000", // 10,000 BTC with 10 decimals
				Decimal: 10,
			},
			want: &model.Web3BigInt{
				Value:   "1000000000000", // 10 with 8 decimals
				Decimal: 8,
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getConversionRatio(tt.circulatedIcy, tt.btcSupply)
			if tt.wantErr != nil {
				assert.Error(t, err)
				assert.Equal(t, tt.wantErr, err)
				assert.Nil(t, got)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, got)
			assert.Equal(t, tt.want.Value, got.Value)
			assert.Equal(t, tt.want.Decimal, got.Decimal)
		})
	}
}
