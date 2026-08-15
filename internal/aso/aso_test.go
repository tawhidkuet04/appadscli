package aso

import (
	"testing"

	"github.com/tawhidkuet04/adastra/internal/itunes"
)

func TestTokens(t *testing.T) {
	got := Tokens("The Best Habit Tracker & Habit App")
	want := []string{"best", "habit", "tracker"}
	if len(got) != len(want) {
		t.Fatalf("Tokens = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestComputeDifficultyEmpty(t *testing.T) {
	d := ComputeDifficulty("x", "us", nil)
	if d.Score != 1 {
		t.Errorf("empty result difficulty = %v, want 1", d.Score)
	}
}

func TestComputeDifficultyScales(t *testing.T) {
	small := []itunes.App{{Name: "A", RatingCount: 50, Rating: 4}}
	big := []itunes.App{{Name: "Habit Tracker", RatingCount: 2_000_000, Rating: 4.8}}
	ds := ComputeDifficulty("habit tracker", "us", small)
	db := ComputeDifficulty("habit tracker", "us", big)
	if ds.Score >= db.Score {
		t.Errorf("small market (%v) should be easier than big (%v)", ds.Score, db.Score)
	}
	if db.Score > 10 || ds.Score < 1 {
		t.Errorf("scores out of bounds: %v %v", ds.Score, db.Score)
	}
	if db.TitleMatches != 1 {
		t.Errorf("TitleMatches = %d, want 1", db.TitleMatches)
	}
}

func TestCompetitorGap(t *testing.T) {
	mine := &itunes.App{Name: "Sleepy", Subtitle: "sleep sounds"}
	comps := []*itunes.App{
		{Name: "DreamWell", Subtitle: "sleep tracker alarm"},
		{Name: "NightOwl", Subtitle: "smart alarm tracker"},
	}
	gaps := CompetitorGap(mine, comps)
	if len(gaps) == 0 {
		t.Fatal("expected gap terms")
	}
	if gaps[0].Term != "alarm" && gaps[0].Term != "tracker" {
		t.Errorf("top gap = %q, want alarm or tracker (both used twice)", gaps[0].Term)
	}
	for _, g := range gaps {
		if g.Term == "sleep" {
			t.Error("'sleep' is in my metadata; must not be a gap")
		}
	}
}

func TestAuditMetadata(t *testing.T) {
	app := &itunes.App{Name: "Focus Timer Pomodoro Deep Work App", Subtitle: "Focus timer for deep work"}
	issues := AuditMetadata(app)
	var sawTitle, sawDup bool
	for _, i := range issues {
		if i.Field == "title" && i.Severity == "error" {
			sawTitle = true
		}
		if i.Field == "subtitle" && i.Severity == "warn" {
			sawDup = true
		}
	}
	if !sawTitle {
		t.Error("expected title-too-long error")
	}
	if !sawDup {
		t.Error("expected subtitle duplication warning")
	}
}
