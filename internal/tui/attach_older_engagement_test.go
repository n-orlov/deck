package tui

import (
	"reflect"
	"testing"

	"github.com/n-orlov/deck/internal/store"
)

// R319/R321/R322: a full attach's trigger is delivered after the attach ends.
// If the row has meanwhile moved to a newer attention episode and THAT episode
// was already engaged (a preview key, a newer attach, the reload's own
// trigger), the delayed older engagement is a no-op: it must not replace the
// newer episode's cached decision or sidebar hold, write anything durable, or
// move the already-engaged row -- whether the newer episode shares the older
// one's stamp or the clock stepped back.
func TestDelayedOlderEngagementCannotDisturbAnEngagedNewerEpisode(t *testing.T) {
	for _, tc := range []struct {
		name string
		move func(t *testing.T, db *store.Store, observedAt int64)
	}{
		// The finding's own two cases.
		{"newer error on the same stamp", func(t *testing.T, db *store.Store, at int64) {
			attachEpisodeMove(t, db, "error", at)
		}},
		{"newer error after the clock stepped back", func(t *testing.T, db *store.Store, at int64) {
			attachEpisodeMove(t, db, "error", at-800)
		}},
		// The same rule, other ways the newer prompt can collide.
		{"newer waiting prompt via running in one millisecond", func(t *testing.T, db *store.Store, at int64) {
			attachEpisodeMove(t, db, "running", at)
			attachEpisodeMove(t, db, "waiting", at)
		}},
		{"newer error several transitions on, back on the observed stamp", func(t *testing.T, db *store.Store, at int64) {
			attachEpisodeMove(t, db, "running", at-300)
			attachEpisodeMove(t, db, "waiting", at-200)
			attachEpisodeMove(t, db, "running", at-100)
			attachEpisodeMove(t, db, "error", at)
		}},
	} {
		for _, trigger := range []engagementTrigger{engageKey, engageScroll, engageScreen} {
			t.Run(tc.name+"/"+string(trigger), func(t *testing.T) {
				rig := engagementFixture(t, SortOrderAttention, "row-waiting", false)
				observed := snapshotDurable(t, rig.db, "row-waiting").Session
				tc.move(t, rig.db, observed.StatusAt)
				m := reloadInert(t, rig.m)
				newer := snapshotDurable(t, rig.db, "row-waiting").Session
				if newer.Episode() == observed.Episode() || newer.StatusSeen() || newer.StatusSeq <= observed.StatusSeq {
					t.Fatalf("fixture: want a newer unseen episode, observed=%+v newer=%+v", observed.Episode(), newer.Episode())
				}
				before := inertOrder(m)

				// The newer episode is engaged first.
				m, issued := finishAttach(t, m, attachFinished{engaged: trigger, observed: newer})
				if !issued {
					t.Fatal("engagement of the newer episode issued no write")
				}
				engaged := snapshotDurable(t, rig.db, "row-waiting")
				if !engaged.Session.StatusSeen() {
					t.Fatalf("fixture: the newer episode is not seen: %+v", engaged.Session)
				}
				orderEngaged := inertOrder(m)
				if !equalIDs(orderEngaged, before) {
					t.Fatalf("engaging the newer episode moved rows: %v -> %v", before, orderEngaged)
				}
				writes := *rig.writes

				// Then the older attach's completion arrives.
				m, issued = finishAttach(t, m, attachFinished{engaged: trigger, observed: observed})
				if issued || *rig.writes != writes {
					t.Fatal("a delayed older engagement issued a write over the engaged newer episode")
				}
				after := snapshotDurable(t, rig.db, "row-waiting")
				if !reflect.DeepEqual(after.Session, engaged.Session) || after.Events != engaged.Events {
					t.Fatalf("a delayed older engagement changed durable state:\n%+v\n%+v", engaged, after)
				}
				if got := m.engagement.issued["row-waiting"]; got != newer.Episode() {
					t.Fatalf("cached episode = %+v, want the newer %+v", got, newer.Episode())
				}
				m = reloadInert(t, m)
				if got := inertOrder(m); !equalIDs(got, before) {
					t.Fatalf("a delayed older engagement moved the engaged row: %v -> %v", before, got)
				}
				// And a repeat of the newer episode's own engagement stays inert.
				if _, again := finishAttach(t, m, attachFinished{engaged: trigger, observed: newer}); again {
					t.Fatal("the newer episode was engaged a second time")
				}
			})
		}
	}
}

// The preview's own trigger (Model.engage) is the same rule: an older
// episode's engageRow after the newer one was engaged does not take over the
// cell.
func TestOlderEpisodeEngageRowKeepsTheNewerEpisodesEntry(t *testing.T) {
	rig := engagementFixture(t, SortOrderAttention, "row-waiting", false)
	observed := snapshotDurable(t, rig.db, "row-waiting").Session
	attachEpisodeMove(t, rig.db, "error", observed.StatusAt)
	newer := snapshotDurable(t, rig.db, "row-waiting").Session
	m := rig.m
	if cmd := m.engageRow(engageKey, newer); cmd == nil {
		t.Fatal("the newer episode was not engaged")
	}
	if cmd := m.engageRow(engageKey, observed); cmd != nil {
		t.Fatal("an older episode was engaged after the newer one")
	}
	if got := m.engagement.issued["row-waiting"]; got != newer.Episode() {
		t.Fatalf("cached episode = %+v, want %+v", got, newer.Episode())
	}
}
