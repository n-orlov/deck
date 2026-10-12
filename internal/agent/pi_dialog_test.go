package agent

import "testing"

// TestPiDialogFocused pins the screens Pi's keystroke fallback must refuse on:
// the selector (ui.select and ui.confirm) and the input dialog, each shaped as
// pi 0.84 renders it, and the screens it must not refuse on.
func TestPiDialogFocused(t *testing.T) {
	rows := []struct {
		name   string
		screen string
		want   bool
	}{
		{"confirm dialog", "────\n\n Allow bash?\n Run rm -rf build\n\n → Yes\n   No\n\n ↑↓ navigate  enter select  escape/ctrl+c cancel\n\n────\n", true},
		{"input dialog", "────\n Session name\n > \n enter submit  escape/ctrl+c cancel\n────\n", true},
		{"idle footer", " ~/w/project\n ↑12k ↓1k $0.010 4.2%/200k (auto) • model\n", false},
		{"running loader", " ⠧ Working...\n Running... (escape/ctrl+c to cancel)\n", false},
		{"navigate hint without cancel", " ↑↓ navigate\n", false},
		{"empty", "", false},
	}
	for _, row := range rows {
		if got := (Pi{}).DialogFocused(row.screen); got != row.want {
			t.Errorf("%s: DialogFocused = %v, want %v", row.name, got, row.want)
		}
	}
	if !IsDialogDetector(NewPi()) {
		t.Fatal("Pi is not a DialogDetector, so the push gate never reads its pane")
	}
}
