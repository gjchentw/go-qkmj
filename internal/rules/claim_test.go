package rules_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/gjchentw/go-qkmj/internal/rules"
)

func TestComputeChowOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		concealed []rules.Tile
		target    rules.Tile
		want      []rules.ChowOption
	}{
		{
			name:      "two chow choices 234 and 345",
			concealed: []rules.Tile{rules.Wan2, rules.Wan3, rules.Wan5},
			target:    rules.Wan4,
			want: []rules.ChowOption{
				{Tiles: [3]rules.Tile{rules.Wan2, rules.Wan3, rules.Wan4}},
				{Tiles: [3]rules.Tile{rules.Wan3, rules.Wan4, rules.Wan5}},
			},
		},
		{
			name:      "three chow choices 345 456 567",
			concealed: []rules.Tile{rules.Suo3, rules.Suo4, rules.Suo6, rules.Suo7},
			target:    rules.Suo5,
			want: []rules.ChowOption{
				{Tiles: [3]rules.Tile{rules.Suo3, rules.Suo4, rules.Suo5}},
				{Tiles: [3]rules.Tile{rules.Suo4, rules.Suo5, rules.Suo6}},
				{Tiles: [3]rules.Tile{rules.Suo5, rules.Suo6, rules.Suo7}},
			},
		},
		{
			name:      "single chow choice at boundary 123",
			concealed: []rules.Tile{rules.Tong2, rules.Tong3},
			target:    rules.Tong1,
			want: []rules.ChowOption{
				{Tiles: [3]rules.Tile{rules.Tong1, rules.Tong2, rules.Tong3}},
			},
		},
		{
			name:      "no cross suit wraparound",
			concealed: []rules.Tile{rules.Wan8, rules.Wan9, rules.Suo2},
			target:    rules.Suo1,
			want:      nil,
		},
		{
			name:      "honor tile has no chow options",
			concealed: []rules.Tile{rules.RedDragon, rules.RedDragon, rules.WhiteDragon},
			target:    rules.RedDragon,
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := rules.ComputeChowOptions(tt.concealed, tt.target)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("ComputeChowOptions() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestClaimWindow_ParallelWaitAndPriority(t *testing.T) {
	t.Parallel()

	chow234 := rules.ChowOption{Tiles: [3]rules.Tile{rules.Wan2, rules.Wan3, rules.Wan4}}
	win := rules.NewClaimWindow(rules.ClaimWindowDiscard, rules.Seat1, rules.Wan4, []rules.SeatClaimOptions{
		{Seat: rules.Seat2, Options: []rules.ClaimKind{rules.ClaimChow}, ChowOptions: []rules.ChowOption{chow234}},
		{Seat: rules.Seat3, Options: []rules.ClaimKind{rules.ClaimPong}},
	})

	prompts := win.Prompts()
	if len(prompts) != 2 {
		t.Fatalf("len(Prompts()) = %d, want 2", len(prompts))
	}
	for _, ev := range prompts {
		if ev.Audience() != rules.AudiencePrivate {
			t.Fatalf("prompt must be private, got %v", ev.Audience())
		}
	}

	// Unprompted Seat4 must be rejected.
	if _, _, err := win.Submit(rules.Seat4, rules.ClaimPass, rules.ChowOption{}); !errors.Is(err, rules.ErrClaimNotPending) {
		t.Fatalf("unprompted seat err = %v, want ErrClaimNotPending", err)
	}

	// Seat2 chooses chow (single option auto-adopted); window must wait for Seat3.
	_, res, err := win.Submit(rules.Seat2, rules.ClaimChow, rules.ChowOption{})
	if err != nil {
		t.Fatalf("Seat2 submit err = %v", err)
	}
	if res != nil {
		t.Fatalf("window resolved prematurely before Seat3 decided: %+v", res)
	}

	// Seat2 cannot change decision once submitted.
	if _, _, err := win.Submit(rules.Seat2, rules.ClaimPass, rules.ChowOption{}); !errors.Is(err, rules.ErrClaimAlreadyDecided) {
		t.Fatalf("duplicate submit err = %v, want ErrClaimAlreadyDecided", err)
	}

	// Seat3 submits pong -> Pong beats Chow.
	_, res, err = win.Submit(rules.Seat3, rules.ClaimPong, rules.ChowOption{})
	if err != nil {
		t.Fatalf("Seat3 submit err = %v", err)
	}
	if res == nil {
		t.Fatal("expected resolution after all prompted seats decided")
	}
	if res.Kind != rules.ClaimPong || res.Winner != rules.Seat3 {
		t.Fatalf("resolution = %+v, want ClaimPong by Seat3", res)
	}
}

func TestClaimWindow_TwoStepChowChoice(t *testing.T) {
	t.Parallel()

	c234 := rules.ChowOption{Tiles: [3]rules.Tile{rules.Wan2, rules.Wan3, rules.Wan4}}
	c345 := rules.ChowOption{Tiles: [3]rules.Tile{rules.Wan3, rules.Wan4, rules.Wan5}}
	win := rules.NewClaimWindow(rules.ClaimWindowDiscard, rules.Seat1, rules.Wan4, []rules.SeatClaimOptions{
		{Seat: rules.Seat2, Options: []rules.ClaimKind{rules.ClaimChow}, ChowOptions: []rules.ChowOption{c234, c345}},
	})

	// Step 1: submit ClaimChow with empty ChowOption -> enters WaitChowChoice.
	events, res, err := win.Submit(rules.Seat2, rules.ClaimChow, rules.ChowOption{})
	if err != nil {
		t.Fatalf("step 1 err = %v", err)
	}
	if res != nil {
		t.Fatalf("should not resolve before chow choice selected: %+v", res)
	}
	if win.WaitingChowSeat() != rules.Seat2 {
		t.Fatalf("WaitingChowSeat() = %v, want Seat2", win.WaitingChowSeat())
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 ChowChoicePromptEvent, got %d", len(events))
	}
	prompt, ok := events[0].(rules.ChowChoicePromptEvent)
	if !ok || prompt.TargetSeat() != rules.Seat2 || len(prompt.Choices) != 2 {
		t.Fatalf("unexpected event: %+v", events[0])
	}

	// Invalid chow choice must fail without leaving WaitChowChoice.
	bad := rules.ChowOption{Tiles: [3]rules.Tile{rules.Wan4, rules.Wan5, rules.Wan6}}
	if _, _, err := win.Submit(rules.Seat2, rules.ClaimChow, bad); !errors.Is(err, rules.ErrInvalidClaimOption) {
		t.Fatalf("invalid chow err = %v, want ErrInvalidClaimOption", err)
	}

	// Step 2: select valid choice 345 -> resolves cleanly.
	_, res, err = win.Submit(rules.Seat2, rules.ClaimChow, c345)
	if err != nil {
		t.Fatalf("step 2 err = %v", err)
	}
	if res == nil || res.Kind != rules.ClaimChow || res.Winner != rules.Seat2 || res.Chow != c345 {
		t.Fatalf("resolution = %+v, want ClaimChow 345 by Seat2", res)
	}
}

func TestClaimWindow_HeadBumpCutWinClockwise(t *testing.T) {
	t.Parallel()

	// TriggerSeat = Seat3; Seat2 and Seat4 both claim Win.
	// Clockwise from Seat3: Seat4 (Next) -> Seat1 -> Seat2. So Seat4 wins!
	win := rules.NewClaimWindow(rules.ClaimWindowDiscard, rules.Seat3, rules.Suo7, []rules.SeatClaimOptions{
		{Seat: rules.Seat2, Options: []rules.ClaimKind{rules.ClaimWin}},
		{Seat: rules.Seat4, Options: []rules.ClaimKind{rules.ClaimWin}},
	})

	if _, res, err := win.Submit(rules.Seat2, rules.ClaimWin, rules.ChowOption{}); err != nil || res != nil {
		t.Fatalf("Seat2 submit = (%+v, %v), want pending", res, err)
	}
	_, res, err := win.Submit(rules.Seat4, rules.ClaimWin, rules.ChowOption{})
	if err != nil {
		t.Fatalf("Seat4 submit err = %v", err)
	}
	if res == nil || res.Kind != rules.ClaimWin || res.Winner != rules.Seat4 {
		t.Fatalf("resolution = %+v, want head-bump ClaimWin by Seat4", res)
	}
	if len(res.PassedWinSeats) != 0 {
		t.Fatalf("both claimed win, PassedWinSeats = %v, want empty", res.PassedWinSeats)
	}
}

func TestClaimWindow_PassedWinTracking(t *testing.T) {
	t.Parallel()

	// Seat2 has Win+Pong and picks Pong; Seat4 has Win and picks Pass.
	// Both declined Win in a Discard window -> both must be reported in PassedWinSeats.
	win := rules.NewClaimWindow(rules.ClaimWindowDiscard, rules.Seat1, rules.Tong5, []rules.SeatClaimOptions{
		{Seat: rules.Seat2, Options: []rules.ClaimKind{rules.ClaimPong, rules.ClaimWin}},
		{Seat: rules.Seat4, Options: []rules.ClaimKind{rules.ClaimWin}},
	})

	if _, _, err := win.Submit(rules.Seat2, rules.ClaimPong, rules.ChowOption{}); err != nil {
		t.Fatalf("Seat2 submit err = %v", err)
	}
	_, res, err := win.Submit(rules.Seat4, rules.ClaimPass, rules.ChowOption{})
	if err != nil {
		t.Fatalf("Seat4 submit err = %v", err)
	}
	if res == nil || res.Kind != rules.ClaimPong || res.Winner != rules.Seat2 {
		t.Fatalf("resolution = %+v, want ClaimPong by Seat2", res)
	}
	if !slices.Contains(res.PassedWinSeats, rules.Seat2) || !slices.Contains(res.PassedWinSeats, rules.Seat4) {
		t.Fatalf("PassedWinSeats = %v, want both Seat2 and Seat4", res.PassedWinSeats)
	}

	// Flower claim declined must NOT record PassedWinSeats.
	flowerWin := rules.NewClaimWindow(rules.ClaimWindowFlower, rules.Seat3, rules.FlowerBamboo, []rules.SeatClaimOptions{
		{Seat: rules.Seat1, Options: []rules.ClaimKind{rules.ClaimWin}, FlowerWinKind: rules.WinSevenRobOne, FlowerPayer: rules.Seat3},
	})
	_, fres, err := flowerWin.Submit(rules.Seat1, rules.ClaimPass, rules.ChowOption{})
	if err != nil {
		t.Fatalf("flower pass err = %v", err)
	}
	if fres == nil || fres.Kind != rules.ClaimPass || len(fres.PassedWinSeats) != 0 {
		t.Fatalf("flower pass resolution = %+v, want ClaimPass with empty PassedWinSeats", fres)
	}
}
