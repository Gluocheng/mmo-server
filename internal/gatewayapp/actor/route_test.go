package actor

import (
	"testing"

	pmessage "github.com/cherry-game/cherry/net/parser/pomelo/message"
)

func TestIsAdminGameRoute(t *testing.T) {
	cases := []struct {
		route string
		want  bool
	}{
		{route: "game.config.reload", want: true},
		{route: "game.gm.reload", want: true},
		{route: "game.GM.reload", want: true},
		{route: "game.bag.list", want: false},
		{route: "game.player.enter", want: false},
		{route: "game.chat.send", want: false},
	}
	for _, tc := range cases {
		r, err := pmessage.DecodeRoute(tc.route)
		if err != nil {
			t.Fatalf("DecodeRoute(%s): %v", tc.route, err)
		}
		if got := isAdminGameRoute(r); got != tc.want {
			t.Fatalf("isAdminGameRoute(%s)=%v want %v", tc.route, got, tc.want)
		}
	}
	if isAdminGameRoute(nil) {
		t.Fatal("nil route should not be admin")
	}
}
