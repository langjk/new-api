package controller

import "testing"

func TestChannelMatchesTargetID(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{`[{"rid": 182533,"old":1}]`, true},
		{`[{"rid":1825330}]`, false},
		{`[{"rid":1,"name":"182533","usd":182533}]`, false},
		{`[{"rid":1},{"rid":182533}]`, true},
		{`null`, false}, {`broken`, false}, {`[]`, false},
	} {
		if got := channelMatchesTargetID(tc.raw, 182533); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.raw, got, tc.want)
		}
	}
}
