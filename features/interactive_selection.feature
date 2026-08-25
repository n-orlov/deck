Feature: Steer 017 item 3 (task 216) -- tmux-like drag-to-copy selection in interactive preview mode

  SPEC §11.8/§11.9's "Selection and copy" (post-395babf): a mouse drag
  beginning inside the interactive preview's own content box selects
  text over the cells deck itself drew (the same grid RenderRows
  renders, including its own scrollback), and releasing writes the
  selected text to a tmux buffer on deck's OWN private server -- the
  load-bearing, tested half of the copy, proven here with a real `tmux
  show-buffer` against the scenario's own socket. A best-effort OSC 52
  write also happens alongside it, but that half is deliberately
  untested by this harness (there is no real outer terminal to observe
  it land in). A plain click, with no motion event in between press and
  release, is a different gesture and commits nothing at all -- SPEC
  §11.8's stated non-exception.

  @task-216-drag-to-copy
  Scenario: dragging over the interactive preview copies the selected text to deck's own tmux buffer
    Given deck client "A" is started
    When deck client "A" creates shell session "drag-copy-target"
    Then deck client "A" screen contains "drag-copy-target"
    And deck client "A" selects session "drag-copy-target"
    When deck client "A" enters interactive mode
    And deck client "A" types "echo DRAGCOPYTOKEN" and Enter into the interactive pane
    Then deck client "A" screen contains "DRAGCOPYTOKEN"
    When deck client "A" drags to select "DRAGCOPYTOKEN" over the line containing it in the interactive pane
    Then deck's own selection buffer eventually contains "DRAGCOPYTOKEN"
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" exits cleanly

  @task-216-drag-to-copy
  Scenario: a plain click (no drag) over the interactive preview commits no selection at all
    Given deck client "A" is started
    When deck client "A" creates shell session "click-nocopy-target"
    Then deck client "A" screen contains "click-nocopy-target"
    And deck client "A" selects session "click-nocopy-target"
    When deck client "A" enters interactive mode
    And deck client "A" types "echo CLICKNOCOPYTOKEN" and Enter into the interactive pane
    Then deck client "A" screen contains "CLICKNOCOPYTOKEN"
    When deck client "A" clicks once (no drag) on the line containing "CLICKNOCOPYTOKEN" in the interactive pane
    Then deck's own selection buffer is empty
    When deck client "A" leaves interactive mode
    Then deck client "A" screen contains "deck - sessions"
    And deck client "A" exits cleanly
