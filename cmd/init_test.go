package cmd

import "testing"

func TestParsePortRange(t *testing.T) {
	valid := map[string][2]int{
		"3001-3999":   {3001, 3999},
		" 1 - 65535 ": {1, 65535},
		"8080-8080":   {8080, 8080},
	}
	for input, want := range valid {
		got, err := parsePortRange(input)
		if err != nil || got.Min != want[0] || got.Max != want[1] {
			t.Errorf("parsePortRange(%q) = %+v, %v; want %v", input, got, err, want)
		}
	}
	for _, input := range []string{"", "3000", "a-b", "0-10", "20-10", "60000-70000", "1-65536"} {
		if got, err := parsePortRange(input); err == nil {
			t.Errorf("parsePortRange(%q) = %+v, want error", input, got)
		}
	}
}
