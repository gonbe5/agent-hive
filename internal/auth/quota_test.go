package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBeginningOfNextMonth(t *testing.T) {
	tests := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{
			name: "普通月份",
			in:   time.Date(2026, 3, 15, 12, 30, 0, 0, time.UTC),
			want: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "12月跨年",
			in:   time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC),
			want: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "月初",
			in:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			want: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "闰年2月",
			in:   time.Date(2024, 2, 29, 12, 0, 0, 0, time.UTC),
			want: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "非UTC时区转换为UTC",
			in:   time.Date(2026, 6, 15, 12, 0, 0, 0, time.FixedZone("CST", 8*3600)),
			want: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := beginningOfNextMonth(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}
