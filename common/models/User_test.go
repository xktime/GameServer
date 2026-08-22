package models

import (
	"testing"
	"time"
)

func TestUserRecordLogin(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	dayOneMorning := time.Date(2026, 8, 21, 9, 0, 0, 0, location).Unix()
	dayOneEvening := time.Date(2026, 8, 21, 23, 0, 0, 0, location).Unix()
	dayTwoMorning := time.Date(2026, 8, 22, 1, 0, 0, 0, location).Unix()
	dayTwoNoon := time.Date(2026, 8, 22, 12, 0, 0, 0, location).Unix()

	tests := []struct {
		name       string
		user       User
		loginTimes []int64
		wantDays   int32
	}{
		{
			name:       "new user starts at one day",
			loginTimes: []int64{dayOneMorning},
			wantDays:   1,
		},
		{
			name: "same-day login does not increment",
			user: User{
				LastOfflineTime: dayOneMorning,
				LoginTime:       dayOneMorning,
				TotalLoginDays:  7,
			},
			loginTimes: []int64{dayOneEvening},
			wantDays:   7,
		},
		{
			name: "cross-day login increments once",
			user: User{
				LastOfflineTime: dayOneEvening,
				LoginTime:       dayOneEvening,
				TotalLoginDays:  7,
			},
			loginTimes: []int64{dayTwoMorning},
			wantDays:   8,
		},
		{
			name: "repeated login on the new day stays idempotent",
			user: User{
				LastOfflineTime: dayOneEvening,
				LoginTime:       dayOneEvening,
				TotalLoginDays:  7,
			},
			loginTimes: []int64{dayTwoMorning, dayTwoNoon},
			wantDays:   8,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, loginTime := range test.loginTimes {
				test.user.RecordLogin(loginTime)
			}
			if test.user.TotalLoginDays != test.wantDays {
				t.Fatalf("总登录天数 = %d，期望 %d", test.user.TotalLoginDays, test.wantDays)
			}
			wantLoginTime := test.loginTimes[len(test.loginTimes)-1]
			if test.user.LoginTime != wantLoginTime {
				t.Fatalf("登录时间 = %d，期望 %d", test.user.LoginTime, wantLoginTime)
			}
		})
	}
}
