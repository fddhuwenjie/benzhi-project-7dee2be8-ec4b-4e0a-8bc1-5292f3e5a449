package domain

import "testing"

func TestTransitionAndRound(t *testing.T) {
	x := &VigorTrial{Status: Draft, Groups: []string{"A"}, Protocol: Protocol{SampleSize: 10, Rounds: 1, Threshold: 50}}
	if err := x.Transition(ProtocolLocked); err != nil {
		t.Fatal(err)
	}
	if err := x.AddRound(GerminationRound{GroupCode: "A", RoundNo: 1, NormalCount: 6, AbnormalCount: 2, UngerminatedCount: 2, Operator: "tech"}); err != nil {
		t.Fatal(err)
	}
	if x.Rounds[0].VigorRate != 60 {
		t.Fatal(x.Rounds[0].VigorRate)
	}
}
