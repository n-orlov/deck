Feature: A deck client's tmux process cost is constant per tick (R184, R185, GH #49)

  Every tmux call deck makes is a fork and exec of a tmux process. Listing
  sessions is one process however many sessions exist, a passive preview
  tick is one process, and an interactive tick on a shell is one
  display-message. This counts the processes a client really spawns, through
  a fixture tmux first on the client's PATH, over a window that opens after
  the twelve sessions exist (and after interactive entry has finished), at
  the product's own default reconcile and preview cadence. The bound is an
  upper bound, so load can only lower the count.

  @requirement-185-tmux-spawn-budget
  Scenario: twelve shell sessions in list mode spawn at most 75 tmux processes in 10 seconds
    Given deck client "A" is started counting its tmux spawns at the product's own tick cadence
    When deck client "A" creates 12 shell sessions named "spawn-list-NN"
    Then deck client "A" screen contains "spawn-list-12"
    When the tmux spawns of deck client "A" are counted over a window of 10 seconds
    Then at most 75 tmux processes were spawned in that window
    And deck client "A" exits cleanly

  @requirement-185-tmux-spawn-budget
  Scenario: twelve shell sessions in interactive mode spawn at most 75 tmux processes in 10 seconds
    Given deck client "A" is started counting its tmux spawns at the product's own tick cadence
    When deck client "A" creates 12 shell sessions named "spawn-int-NN"
    Then deck client "A" screen contains "spawn-int-12"
    And deck client "A" selects session "spawn-int-01"
    When deck client "A" enters interactive mode
    Then deck client "A" screen contains "Ctrl+Q"
    And tmux pane pipe is armed for session "spawn-int-01"
    And tmux window "deck_spawn-int-01" option "@deck_isize_owner" is set in the window scope
    When the tmux spawns of deck client "A" are counted over a window of 10 seconds
    Then at most 75 tmux processes were spawned in that window
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" exits cleanly
