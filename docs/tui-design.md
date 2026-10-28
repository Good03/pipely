# Pipely TUI design

This is the design reference for TUI changes. Extend the shared components rather
than introducing a new style for each screen.

## Colors

| Role | Color | Use |
| --- | --- | --- |
| Workspace | `#171522` | Main application background |
| Dialog surface | `#30263e` | Dialog body, table cells, inactive buttons |
| Accent | `#b694ff` | Borders, titles, focused selection |
| Primary text | `#eee8fa` | Values, options, button labels |
| Secondary text | `#c9bedb` | Labels and supporting metadata |

Use charcoal text on a purple selection. Reserve semantic error and warning
colors for diagnostics and logs. Do not use green, yellow, or white blocks as
default selection styles.

## Dialogs

- Center configuration forms and option pickers. Keep browsing, monitoring, and
  logs in the main workspace.
- Use one outer purple border and a short title. Keep all content on one uniform
  dialog surface. No nested frames, dark table-cell blocks, or separate panels
  around action rows.
- Use the shared `centeredDialog` in `internal/pipely/tui_dialog.go`.
- Help, errors, diagnostics, links, and confirmations use this same component.
  Message dialogs use a scrollable text view with the shared bottom action row;
  do not introduce `tview.Modal` with its independent sizing rules.
- Size to content, cap width at 104 columns, and shrink with the terminal.
  Keep two columns of horizontal margin and one row of vertical margin where
  space permits. Inner padding is one row and two columns.
- Scroll long lists, tables, and forms to keep their focused item visible.
- Show the primary action first, followed by secondary or destructive actions.
  Align action buttons together at the bottom using the shared button styles.
- Inactive buttons use the dialog surface. Only the focused button receives a
  purple fill. Do not add permanent dark rectangles or shortcut text to buttons.

Group details use a shared branch dropdown above a member table and a bottom action
row: **Run group**, **Delete**.
The table contains pipelines only. Arrow Down from its last member enters the
actions; Left/Right switches buttons; Up returns to members. Up from the first
member enters the branch dropdown; Enter opens the choices and confirms a branch.
Escape closes the choices before leaving the group. Branch choices are loaded
from member repositories, deduplicated, and sorted. Keep saved branch choices
available while loading or if discovery fails.

Organization/project selection lists contexts only, with **Add organization** in
the bottom action row. Project pickers list projects only. Do not put actions or
Back entries into option lists; Escape handles returning to the previous view.

## Selection and focus

- Highlight only the control with keyboard focus. Preserve inactive cursor
  positions without retaining their filled selection.
- Branch and parameter dropdowns use charcoal options with light text and a
  purple selected option. The closed field does not remain highlighted while
  its option list is open.
- Use arrows and Vim navigation in lists/tables, Space for toggles, and Enter
  for selection or the current action. Tab is optional.
- Escape closes an open dropdown or overlay first, then returns to the previous
  dialog or workspace. Keep focus inside a choice dialog until it is closed.
- Skip informational form text and disabled actions. Do not focus a read-only
  TextView through its automatic completion callback; this can deadlock tview.
- Every container must forward keyboard, paste, and mouse events to its content.

## Group configuration

- Select the branch once from the dropdown in the group window. Save it before member editing or
  launch; apply it to every member for metadata, preview, and submission. Persist
  it across restarts and membership changes without resetting stages or parameters.
- Member settings show the effective branch as read-only text. Parameters and
  stages remain specific to each pipeline. Existing groups without a shared
  branch retain saved member branches or Azure defaults until one is set.
- Enter on a member opens its settings. Enter in stage selection saves that
  member and returns to the group. Editing a member never queues a pipeline.
- Run group opens the group review and confirmation before submission.
- Display saved branches and stages. Use **Azure default branch** and **All
  stages** for unset values; never show empty labels.
- Persist saved member settings and preserve them when updating membership.

## Text

Use concise labels, configuration values, counts, progress, and actionable errors.
Keep keyboard shortcuts in the footer and Help. Avoid tutorial sentences,
implementation details, and explanations of basic pipeline behavior in dialogs.
Confirmation dialogs identify the action and its targets; they do not add
generic descriptions of how Azure works.
Keep shortcuts out of dialog titles. Put them in the contextual footer.

## Validation

Reuse `dialogBackground`, `dialogButtonStyle`, `dialogSelectedStyle`,
`styleDialogTable`, and `centeredDialog` rather than copying color literals.
For changes to layout or interaction, check 80×24 and 120×35 terminals, long
content, focus transitions, and Escape behavior. Route simulated input through
the root layout; sending directly to a focused widget misses container bugs.
