/*---------------------------------------------------------------------------------------------
 *  Copyright (c) IBAX. All rights reserved.
 *  See LICENSE in the project root for license information.
 *--------------------------------------------------------------------------------------------*/
package smart

import (
	"testing"
)

func TestDateTimeLocation(t *testing.T) {
	type args struct {
		unix         int64
		locationName string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{"Error", args{0, "Location/Bad"}, "", true},
		{"Chongqing", args{1562032800, "Asia/Chongqing"}, "2019-07-02 10:00:00", false},
		{"Tokyo", args{1562032800, "Asia/Tokyo"}, "2019-07-02 11:00:00", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DateTimeLocation(tt.args.unix, tt.args.locationName)
			if (err != nil) != tt.wantErr {
				t.Errorf("DateTimeLocation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("DateTimeLocation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnixDateTimeLocation(t *testing.T) {
	type args struct {
		value        string
		locationName string
	}
	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr bool
	}{
		{"BadLocation", args{"", "Location/Bad"}, 0, true},
		{"BadFormat", args{"2019-07-02", "Asia/Chongqing"}, 0, true},
		{"Chongqing", args{"2019-07-02 10:00:00", "Asia/Chongqing"}, 1562032800, false},
		{"Tokyo", args{"2019-07-02 11:00:00", "Asia/Tokyo"}, 1562032800, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := UnixDateTimeLocation(tt.args.value, tt.args.locationName)
			if (err != nil) != tt.wantErr {
				t.Errorf("UnixDateTimeLocation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("UnixDateTimeLocation() = %v, want %v", got, tt.want)
			}
		})
	}
}
