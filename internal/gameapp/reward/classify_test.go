package reward

import "testing"

func TestClassifySoloIgnoresSingleMemberParty(t *testing.T) {
	out := Classify(1, []Fighter{{
		UID: 1, PlayerID: 11, Damage: 50, At: 10, OnLine: true, PartyID: 9,
	}})
	if out.Class != "solo" || !out.Solo[1] || len(out.Party) != 0 || out.Rank[1] != 1 || out.Last != 1 {
		t.Fatalf("%+v", out)
	}
}

func TestClassifyPartyRequiresTwoOnlineMembers(t *testing.T) {
	out := Classify(2, []Fighter{
		{UID: 1, Damage: 10, At: 1, OnLine: true, PartyID: 8},
		{UID: 2, Damage: 20, At: 2, OnLine: true, PartyID: 8},
		{UID: 3, Damage: 30, At: 3, OnLine: true},
	})
	if out.Class != "party" || !out.Party[1] || !out.Party[2] || out.Party[3] || out.Solo[3] {
		t.Fatalf("%+v", out)
	}
	if out.Rank[3] != 1 || out.Rank[2] != 2 || out.Rank[1] != 3 || out.Last != 2 {
		t.Fatalf("rank %+v last %d", out.Rank, out.Last)
	}
}

func TestClassifyOpenWhenNoQualifiedParty(t *testing.T) {
	out := Classify(2, []Fighter{
		{UID: 1, Damage: 10, At: 1, OnLine: true},
		{UID: 2, Damage: 10, At: 2, OnLine: true, PartyID: 4},
	})
	if out.Class != "open" || len(out.Solo) != 0 || len(out.Party) != 0 {
		t.Fatalf("%+v", out)
	}
	if out.Rank[1] != 1 || out.Rank[2] != 2 {
		t.Fatalf("same damage earlier uid wins? %+v", out.Rank)
	}
}

func TestClassifyPartySizeThenDamageThenID(t *testing.T) {
	bigger := Classify(1, []Fighter{
		{UID: 1, Damage: 1, At: 1, OnLine: true, PartyID: 2},
		{UID: 2, Damage: 1, At: 2, OnLine: true, PartyID: 2},
		{UID: 3, Damage: 9, At: 3, OnLine: true, PartyID: 5},
		{UID: 4, Damage: 9, At: 4, OnLine: true, PartyID: 5},
		{UID: 5, Damage: 9, At: 5, OnLine: true, PartyID: 5},
	})
	if bigger.Class != "party" || !bigger.Party[3] || bigger.Party[1] {
		t.Fatalf("size %+v", bigger.Party)
	}
	tied := Classify(1, []Fighter{
		{UID: 1, Damage: 10, At: 1, OnLine: true, PartyID: 7},
		{UID: 2, Damage: 10, At: 2, OnLine: true, PartyID: 7},
		{UID: 3, Damage: 30, At: 3, OnLine: true, PartyID: 3},
		{UID: 4, Damage: 1, At: 4, OnLine: true, PartyID: 3},
	})
	if !tied.Party[3] || tied.Party[1] {
		t.Fatalf("damage %+v", tied.Party)
	}
	id := Classify(1, []Fighter{
		{UID: 1, Damage: 5, At: 1, OnLine: true, PartyID: 9},
		{UID: 2, Damage: 5, At: 2, OnLine: true, PartyID: 9},
		{UID: 3, Damage: 5, At: 3, OnLine: true, PartyID: 4},
		{UID: 4, Damage: 5, At: 4, OnLine: true, PartyID: 4},
	})
	if !id.Party[3] || id.Party[1] {
		t.Fatalf("id %+v", id.Party)
	}
}

func TestClassifyOfflineGetsNothing(t *testing.T) {
	out := Classify(2, []Fighter{
		{UID: 1, Damage: 100, At: 1, OnLine: true, PartyID: 1},
		{UID: 2, Damage: 80, At: 2, OnLine: false, PartyID: 1},
		{UID: 3, Damage: 70, At: 3, OnLine: true, PartyID: 1},
	})
	if out.Class != "party" || out.Party[2] || !out.Party[1] || !out.Party[3] {
		t.Fatalf("%+v", out)
	}
	if _, ok := out.Rank[2]; ok || out.Last != 0 {
		t.Fatalf("offline rank %+v last %d", out.Rank, out.Last)
	}
	if out.Rank[1] != 1 || out.Rank[3] != 2 {
		t.Fatalf("rank %+v", out.Rank)
	}
}

func TestClassifyDeadOnLineStillRanks(t *testing.T) {
	out := Classify(2, []Fighter{
		{UID: 2, Damage: 5, At: 9, OnLine: true},
		{UID: 8, Damage: 5, At: 3, OnLine: true},
	})
	if out.Class != "open" || out.Rank[8] != 1 || out.Rank[2] != 2 || out.Last != 2 {
		t.Fatalf("%+v", out)
	}
}
