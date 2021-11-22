package inventory

import "testing"

func TestPaginationValidate(t *testing.T) {
	tests := []struct {
		name       string
		pagination Pagination
		wantErr    bool
	}{
		{
			name: "valid_begin",
			pagination: Pagination{
				Limit:  10,
				Offset: 0,
			},
		},
		{
			name: "one",
			pagination: Pagination{
				Limit:  1,
				Offset: 0,
			},
		},
		{
			name: "valid_high",
			pagination: Pagination{
				Limit:  150,
				Offset: 240,
			},
		},
		{
			name: "negative_limit",
			pagination: Pagination{
				Limit:  -10,
				Offset: 0,
			},
			wantErr: true,
		},
		{
			name: "negative_offset",
			pagination: Pagination{
				Limit:  10,
				Offset: -10,
			},
			wantErr: true,
		},
		{
			name: "negative_both",
			pagination: Pagination{
				Limit:  -10,
				Offset: -5,
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.pagination.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Pagination.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
