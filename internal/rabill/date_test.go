package rabill

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAddMonthsClampsToTheEndOfTheMonth(t *testing.T) {
	cases := []struct {
		name   string
		from   Date
		months int
		want   Date
	}{
		{"plain month", NewDate(2026, time.April, 15), 1, NewDate(2026, time.May, 15)},
		{"a year of a defect-liability period", NewDate(2026, time.April, 30), 12, NewDate(2027, time.April, 30)},
		{"31 January plus one month is 28 February", NewDate(2026, time.January, 31), 1, NewDate(2026, time.February, 28)},
		{"31 January plus one month is 29 February in a leap year", NewDate(2028, time.January, 31), 1, NewDate(2028, time.February, 29)},
		{"31 March plus one month is 30 April", NewDate(2026, time.March, 31), 1, NewDate(2026, time.April, 30)},
		{"31 August plus six months", NewDate(2025, time.August, 31), 6, NewDate(2026, time.February, 28)},
		{"29 February plus twelve months is 28 February", NewDate(2024, time.February, 29), 12, NewDate(2025, time.February, 28)},
		{"29 February plus forty-eight months is 29 February", NewDate(2024, time.February, 29), 48, NewDate(2028, time.February, 29)},
		{"31 December plus one month", NewDate(2026, time.December, 31), 1, NewDate(2027, time.January, 31)},
		{"31 December plus two months", NewDate(2026, time.December, 31), 2, NewDate(2027, time.February, 28)},
		{"across a year boundary", NewDate(2026, time.November, 15), 3, NewDate(2027, time.February, 15)},
		{"eighteen months", NewDate(2026, time.August, 31), 18, NewDate(2028, time.February, 29)},
		// 2026 is not a leap year, so NewDate normalises 29 February to 1 March
		// before the months are added — a date that does not exist never
		// survives into a bill.
		{"a 29 February that does not exist", NewDate(2026, time.February, 29), 0, NewDate(2026, time.March, 1)},
		{"zero months changes nothing", NewDate(2026, time.April, 15), 0, NewDate(2026, time.April, 15)},
		{"backwards one month", NewDate(2026, time.March, 31), -1, NewDate(2026, time.February, 28)},
		{"backwards twelve months", NewDate(2026, time.May, 10), -12, NewDate(2025, time.May, 10)},
		{"backwards across a year", NewDate(2026, time.January, 31), -2, NewDate(2025, time.November, 30)},
		{"backwards thirteen months", NewDate(2026, time.January, 31), -13, NewDate(2024, time.December, 31)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.from.AddMonths(c.months); got != c.want {
				t.Errorf("%s.AddMonths(%d) = %s, want %s", c.from, c.months, got, c.want)
			}
		})
	}
}

func TestAddMonthsLeavesTheZeroDateAlone(t *testing.T) {
	if got := (Date{}).AddMonths(12); !got.IsZero() {
		t.Errorf("the zero date plus twelve months = %s, want the zero date", got)
	}
	if got := (Date{}).AddDays(30); !got.IsZero() {
		t.Errorf("the zero date plus thirty days = %s, want the zero date", got)
	}
}

func TestAddDays(t *testing.T) {
	cases := []struct {
		name string
		from Date
		days int
		want Date
	}{
		{"thirty-day payment period", NewDate(2026, time.April, 15), 30, NewDate(2026, time.May, 15)},
		{"forty-five days across a month end", NewDate(2026, time.January, 20), 45, NewDate(2026, time.March, 6)},
		{"forty-five days across a leap February", NewDate(2028, time.January, 20), 45, NewDate(2028, time.March, 5)},
		{"across a year end", NewDate(2026, time.December, 20), 30, NewDate(2027, time.January, 19)},
		{"28 February in a leap year", NewDate(2028, time.February, 28), 1, NewDate(2028, time.February, 29)},
		{"28 February in a common year", NewDate(2026, time.February, 28), 1, NewDate(2026, time.March, 1)},
		{"a century year that is not a leap year", NewDate(2100, time.February, 28), 1, NewDate(2100, time.March, 1)},
		{"a four-hundred year that is a leap year", NewDate(2000, time.February, 28), 1, NewDate(2000, time.February, 29)},
		{"zero days", NewDate(2026, time.April, 15), 0, NewDate(2026, time.April, 15)},
		{"backwards", NewDate(2026, time.March, 1), -1, NewDate(2026, time.February, 28)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.from.AddDays(c.days); got != c.want {
				t.Errorf("%s.AddDays(%d) = %s, want %s", c.from, c.days, got, c.want)
			}
		})
	}
}

func TestDaysBetween(t *testing.T) {
	cases := []struct {
		name     string
		from, to Date
		want     int
	}{
		{"same day", NewDate(2026, time.April, 1), NewDate(2026, time.April, 1), 0},
		{"a month", NewDate(2026, time.April, 1), NewDate(2026, time.May, 1), 30},
		{"a leap February", NewDate(2028, time.February, 1), NewDate(2028, time.March, 1), 29},
		{"a common February", NewDate(2026, time.February, 1), NewDate(2026, time.March, 1), 28},
		{"a whole leap year", NewDate(2028, time.January, 1), NewDate(2029, time.January, 1), 366},
		{"a whole common year", NewDate(2026, time.January, 1), NewDate(2027, time.January, 1), 365},
		{"backwards", NewDate(2026, time.May, 1), NewDate(2026, time.April, 1), -30},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DaysBetween(c.from, c.to); got != c.want {
				t.Errorf("DaysBetween(%s, %s) = %d, want %d", c.from, c.to, got, c.want)
			}
		})
	}
}

func TestParseDate(t *testing.T) {
	want := NewDate(2026, time.April, 30)
	for _, in := range []string{"2026-04-30", "30/04/2026", "30-04-2026", "30.04.2026", "  2026-04-30  "} {
		got, err := ParseDate(in)
		if err != nil {
			t.Fatalf("ParseDate(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("ParseDate(%q) = %s, want %s", in, got, want)
		}
	}

	if got, err := ParseDate(""); err != nil || !got.IsZero() {
		t.Errorf("ParseDate(\"\") = %s, %v; want the zero date and no error", got, err)
	}

	for _, in := range []string{"next Tuesday", "2026-13-01", "2026-02-30", "30 April 2026"} {
		if got, err := ParseDate(in); err == nil {
			t.Errorf("ParseDate(%q) = %s, want an error", in, got)
		}
	}
}

func TestDateOrdering(t *testing.T) {
	early := NewDate(2026, time.March, 31)
	late := NewDate(2026, time.April, 1)

	if !early.Before(late) {
		t.Error("31 March should fall before 1 April")
	}
	if !late.After(early) {
		t.Error("1 April should fall after 31 March")
	}
	if early.Before(early) || early.After(early) {
		t.Error("a date is neither before nor after itself")
	}
	if !early.Equal(NewDate(2026, time.March, 31)) {
		t.Error("the same day should compare equal")
	}
}

func TestDateJSON(t *testing.T) {
	type holder struct {
		On Date `json:"on"`
	}

	body, err := json.Marshal(holder{On: NewDate(2026, time.April, 30)})
	if err != nil {
		t.Fatalf("marshalling: %v", err)
	}
	if string(body) != `{"on":"2026-04-30"}` {
		t.Errorf("marshalled as %s", body)
	}

	body, err = json.Marshal(holder{})
	if err != nil {
		t.Fatalf("marshalling the zero date: %v", err)
	}
	if string(body) != `{"on":null}` {
		t.Errorf("the zero date marshalled as %s, want null", body)
	}

	for _, raw := range []string{`{"on":"2026-04-30"}`, `{"on":"30/04/2026"}`} {
		var out holder
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			t.Fatalf("unmarshalling %s: %v", raw, err)
		}
		if out.On != NewDate(2026, time.April, 30) {
			t.Errorf("%s gave %s", raw, out.On)
		}
	}

	for _, raw := range []string{`{"on":null}`, `{"on":""}`} {
		var out holder
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			t.Fatalf("unmarshalling %s: %v", raw, err)
		}
		if !out.On.IsZero() {
			t.Errorf("%s gave %s, want the zero date", raw, out.On)
		}
	}

	var out holder
	if err := json.Unmarshal([]byte(`{"on":"not a date"}`), &out); err == nil {
		t.Error("a string that is not a date should be refused")
	}
}

func TestDateLong(t *testing.T) {
	if got := NewDate(2026, time.April, 30).Long(); got != "30 April 2026" {
		t.Errorf("Long() = %q, want \"30 April 2026\"", got)
	}
	if got := (Date{}).Long(); got != "" {
		t.Errorf("the zero date's Long() = %q, want \"\"", got)
	}
}
