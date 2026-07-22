package ops

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestApplyUserInfoCardPremium(t *testing.T) {
	card := map[string]any{"id": int64(1), "type": "user", "title": "Alice"}
	u := &tg.User{ID: 1, FirstName: "Alice", Premium: true}

	applyUserInfoCard(card, u)

	if card["premium"] != true {
		t.Fatalf("premium user: want premium=true, got card=%+v", card)
	}
	// Existing fields unaffected.
	if card["id"] != int64(1) || card["type"] != "user" || card["title"] != "Alice" {
		t.Fatalf("base fields changed: %+v", card)
	}
	if _, ok := card["bot"]; ok {
		t.Fatalf("non-bot must not set bot: %+v", card)
	}
}

func TestApplyUserInfoCardNonPremium(t *testing.T) {
	card := map[string]any{"id": int64(2), "type": "user", "title": "Bob"}
	u := &tg.User{ID: 2, FirstName: "Bob"}

	applyUserInfoCard(card, u)

	if _, ok := card["premium"]; ok {
		t.Fatalf("non-premium must omit premium: %+v", card)
	}
}

func TestApplyUserInfoCardBot(t *testing.T) {
	card := map[string]any{"id": int64(3), "type": "user", "title": "HelperBot"}
	u := &tg.User{ID: 3, FirstName: "HelperBot", Bot: true}

	applyUserInfoCard(card, u)

	if card["bot"] != true {
		t.Fatalf("bot: want bot=true, got card=%+v", card)
	}
	if _, ok := card["premium"]; ok {
		t.Fatalf("non-premium bot must omit premium: %+v", card)
	}
}

func TestApplyUserInfoCardPhoneAndFlags(t *testing.T) {
	card := map[string]any{"id": int64(4), "type": "user", "title": "Carol"}
	u := &tg.User{ID: 4, FirstName: "Carol", Premium: true}
	u.SetPhone("15551234567")

	applyUserInfoCard(card, u)

	if card["premium"] != true {
		t.Fatalf("want premium=true: %+v", card)
	}
	if card["phone"] != "15551234567" {
		t.Fatalf("want phone set: %+v", card)
	}
	if _, ok := card["bot"]; ok {
		t.Fatalf("non-bot must not set bot: %+v", card)
	}
}
