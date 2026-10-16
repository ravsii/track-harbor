---
name: design
description: Use when creating, reviewing, or changing TrackHarbor UI elements, styles, layouts, typography, colors, icons, spacing, or interactive states.
---

# UI Design

Use one consistent visual language across the application. Reuse the elements
and page-composition contracts below instead of inventing a style for each
screen. A page varies its content and actions, not its typography, control
geometry, spacing, or surface treatment. This guide defines the target standard;
apply it through reviewed, focused UI changes, not an automatic application-wide
rewrite. Updating this guide does not approve application changes.

## Workflow

1. Understand the existing element, behavior, and dependencies first.
2. Before changing appearance, create separate temporary HTML previews with
   dummy content. Show both themes, desktop and narrow layouts, and relevant
   hover, keyboard-focus, disabled, and error states to the user.
   For a shared-design change, include every affected page using the same CSS
   and element recipes. Put preview navigation and state selectors outside the
   application frame; illustrate states in their real form or list, not in a
   demonstration section added to the product page.
   During incremental review, show only pages still under review. Retain approved
   designs for implementation without repeatedly presenting them. Implement the
   page the user selects; approval of several previews is not a request to roll
   out every page at once unless they explicitly request the complete redesign.
3. Apply the reviewed design without changing unrelated behavior. Delete
   preview files and preview-only assets afterward; keep them available during
   review, and never commit them.

Documentation-only and appearance-neutral backend changes need no preview.
Prioritize usable controls and content over decorative copy or illustrations.

## Shared page composition

The visual baseline is the existing centered header/player, quiet left
navigation, and a single main content surface. Use this hierarchy on every
authenticated page:

1. **Page header:** one left-aligned `h1`, optional count beside it, and optional
   page-level actions aligned to the trailing edge. Tracks has a heading too.
   Counts use secondary `13px` text; they are not badges or uppercase captions.
2. **Content sections:** stack full-width sections with a `24px` gap. Each has
   an optional `h2`/trailing status row, then content after `18px`. Between
   adjacent sections, use one `1px` border and `24px` top padding on the next
   section. Do not add a separator immediately under the page header.
3. **Section content:** use shared forms, action groups, feedback, and lists.
   A form belongs to its section, not to an extra bordered card. The main
   surface supplies the background, border, radius, and padding once.

Use `24px` main-surface padding (`22px` at ≤ `700px`), within the shared outer
page gutters. Content starts at the top and grows naturally; use document
scrolling instead of a fixed-height scrolling track list. Keep the main column
and section width consistent across pages. Small forms may use multiple columns
with `12px` gaps; stack them at ≤ `700px`.

No numbered overlines such as `01 / CONNECTION`, oversized collection headings,
decorative footer slogans, nested panel borders, or source-specific font/color
themes. Use short task labels: “Connection”, “Your tracks”, “Add a video”.
Explain behavior only when it helps complete an action or recover from failure.

### Page recipes

These are content variations of the same composition, not separate designs.

| Page | Composition |
| --- | --- |
| Tracks | Page header/count, shared media list with source, personal added date, duration and actions. |
| Server tracks | Page header/count, the same media list with source and download readiness instead of a personal added date. Preserve shared-catalog semantics. |
| Spotify / VK Music | Identical source-page structure: Connection heading/status; Sync library and Check connection actions; concise feedback; credentials disclosure when connected, visible form when disconnected; Your tracks section. Only the credential fields and authorization instructions differ. |
| YouTube | Page header/count; Add a video form; Your videos list. Keep this page to adding videos and showing the user's collection. |
| Downloads | Download control section with start/stop and status; My downloads; Shared download queue. Both lists show only unfinished downloads and use the shared media cell/status recipe. Queued, active, stopped and failed items remain visible; completed/downloaded items leave both lists and their counts. Preserve personal/shared boundaries and existing action permissions. |
| Settings | Users, Public proxy sources, Personal proxies, in one vertical stack of equal sections. Reuse the same form/action/list recipes in all three. A user's name, Change password and Delete actions share one row; reveal the password editor below only on request. Personal proxies have a Ping action per row, a Ping all section action, and individual check results. |
| Sign in | Centered surface, `380px` maximum width; brand/theme row, page heading, form and inline feedback. Use the same elements as inside the application, without the player/sidebar. |

## Shared implementation contract

### Canonical implementation

| Location | Responsibility |
| --- | --- |
| `internal/web/assets/app.css` | Shared element geometry, typography, states, lists and navigation/player appearance. This is the only place to define these primitives. |
| `internal/web/assets/theme.css` | Semantic palette and theme selection only. No page overrides or duplicate component rules. |
| `internal/web/tailwind.css` | Semantic utility aliases and explicit template/script sources; generated output is `assets/tailwind.css`. |
| `internal/web/view_components.go` | `media-cell`, `source-label`, `track-status`, `track-list`, and the download-row template. Browser-created download rows clone the same markup. |
| `internal/web/views.go`, `internal/web/view_settings.go` | Page composition and existing behavior hooks. |
| `internal/web/assets/login.css` | Sign-in composition only; reuse the main app's elements. |
| `internal/web/assets/ui.js` | Navigation disclosure, password-editor disclosure and HTMX form feedback. Theme behavior remains in `theme-toggle.js`. |

**For every new element:** find its recipe here, reuse the class/template below,
then compose it in the page. When a genuinely new primitive is needed, define
it once in the canonical files, document its recipe here, and review it in
both themes and narrow layouts. Do not copy a page's controls into a new set of
source-specific styles. Add new Go template/script files to Tailwind's explicit
`@source` list and regenerate assets with `npm run assets:build`.

| Element | Reuse |
| --- | --- |
| Content surface and sections | `.surface.content`, `.page-heading`, `.page-identity`, `.page-count`, `.content-section`, `.section-heading` |
| Forms | `.form`, `.field`, `.actions`; responsive field columns use Tailwind |
| Actions | `.button`, `.button.primary`, `.button.danger`, `.icon-button`; `.responsive-action` with `.action-label` for user/proxy actions |
| Messages | `.helper`, `.meta`, `.feedback`; `data-error="true"` and `role="alert"` for errors |
| Status | `.status` with `data-state="ready|pending|failed"`; track statuses use the `track-status` template and `data-stage` |
| Media | `track-list`/`media-cell` templates; `.media-list` variants `.personal`, `.catalog`, `.queue` supply columns, not new row styles |
| Utility rows | `.user-list`, `.user-item`, `.user-summary`, `.password-editor`; `.proxy-list`, `.proxy-row`, `.proxy-identity` |
| Empty/disclosure | `.empty`, `.disclosure`; keep real titles and recovery text inside these shared elements |

Preserve JS/HTMX IDs and `data-*` hooks when composing shared elements. User
forms use `data-feedback` to target a shared `.feedback` region. Include native
pending/disabled behavior and accessible labels in the first implementation,
not in a later page-specific correction.

Define each visual primitive once: page header, section header, field, primary/
secondary/icon button, action group, disclosure, media cell/row, status, feedback
and empty state. Reuse shared template partials and CSS classes across pages.
Express layout and responsive grid placement with Tailwind; express element
appearance with the shared recipes. Source-specific selectors may arrange
content but must not redefine the size, font, palette or radius of a primitive.

When standardizing an element, replace its conflicting legacy rules instead
of appending a higher-specificity override for each page. In particular, do not
use broad rules such as `#content .grid`, `#content .panel button` or global
`form label` rules to impose a second density system. Keep one canonical token
for each semantic role; errors and selected/hover surfaces must not acquire
page-specific color aliases. Keep Tailwind Preflight disabled.

## Typography

Use `system-ui, sans-serif` throughout the UI. Do not mix font families between
pages, depend on remote fonts, or scale text with viewport width.

| Role | Size / line-height | Weight | Letter-spacing |
| --- | --- | --- | --- |
| Body, inputs, buttons | `15px / 1.5` | `400`; buttons `600` | `normal` |
| Primary heading | `25px / 1.2` | `600` | `-0.7px` |
| Secondary heading | `21px / 1.3` | `600` | `normal` |
| Labels, helper text, errors | `13px / 1.5` | `400` | `normal` |
| Metadata, tooltips | `12px / 1.5` | `400` | `normal` |
| Brand wordmark | `21px / 1.5` | `650` | `-0.6px` |

Use weight `600` for emphasis rather than changing font size. Keep heading and
label alignment left; avoid uppercase, widely spaced form labels.

## Geometry and spacing

Use `box-sizing: border-box`. Sizes include padding and borders. Fixed tokens
remain fixed across themes and viewport sizes; available widths may be fluid.

| Element or relationship | Value |
| --- | --- |
| Input, select, text button minimum height | `40px` |
| Input, select, textarea padding | `8px 10px` |
| Text button padding | `8px 12px` |
| Icon button size / icon size | `32px × 32px` / `18px × 18px` |
| Control radius / border | `6px` / `1px solid var(--edge)` |
| Surface radius / border | `10px` / `1px solid var(--edge)` |
| Surface padding | `24px`; `22px` at widths ≤ `700px` |
| Page horizontal padding | `24px`; `20px` at widths ≤ `700px` |
| Label-to-control / helper-to-control gap | `5px` |
| Form field and action group gap | `12px` |
| Inline element / brand-to-mark gap | `10px` |
| Button icon-to-label gap | `6px` |
| Heading-to-content / section-to-section gap | `18px` / `24px` |
| List row padding / column gap | `8px 10px` / `10px` |
| Compact toolbar padding | `8px 20px`; `8px 12px` at widths ≤ `700px` |
| Adjacent toolbar icon gap / group gap | `4px` / `12px` |
| Range control hit area height / track height / thumb size | `24px` / `3px` / `10px` |
| Keyboard focus outline / offset | `2px solid var(--accent)` / `3px` |

Reset browser heading and paragraph margins; explicitly use these spacing
tokens. Do not halve values, add arbitrary per-page offsets, or use `clamp()`
for typography and element spacing. New exceptions require user review and
an update to this guide, not an undocumented one-off override.

## Palette

Use semantic CSS variables, never hard-coded component colors. Tailwind
utilities must use the existing semantic tokens in `internal/web/tailwind.css`.

| Role / CSS variable | Light | Dark |
| --- | --- | --- |
| Page background `--bg` | `#F4F2ED` | `#0B1522` |
| Recessed surface `--sidebar` | `#E9E8E5` | `#101E2D` |
| Surface `--panel` | `#FFFFFF` | `#172638` |
| Primary text `--ink` | `#202633` | `#EDF6F7` |
| Secondary text `--muted` | `#596376` | `#AEC3CB` |
| Border `--edge` | `#D4D8DF` | `#315063` |
| Accent `--accent` | `#5DD6D0` | `#5DD6D0` |
| Text on accent `--accent-ink` | `#08292C` | `#08292C` |
| Selected background `--selected` | `#DCF3F1` | `#193D47` |
| Error (semantic error token) | `#A12622` | `#FFB4AB` |

Turquoise text is not suitable for small labels on light backgrounds. Use
primary text with an accent border, underline, or background instead.

## Element recipes

**Input, select, textarea:** fill the available width; use page background,
primary text, and the control geometry above. Labels remain visible above
controls; placeholders never replace them. Use the correct input type and
autocomplete purpose. Textareas resize vertically. Hover changes the border
to secondary text color; keyboard focus uses the shared outline.

**Primary button:** accent background and border, dark accent-ink text,
centered label, weight `600`; hover brightness `0.93`. Use the shared control
geometry. Buttons fit their content by default; full-width buttons are an
explicit layout decision, not a different button style.

**Secondary button:** surface background, primary text, standard border;
hover uses selected background. **Icon button:** shared square geometry,
zero padding, centered icon, secondary text; hover uses selected background
and primary text. Give every icon-only button an accessible action label.

**Compact toolbar icons:** use the same `32px` buttons and `18px` icons,
but no border or permanent background. Arrange related actions together with
the toolbar gaps above; show a background only on hover or keyboard focus.
An active toggle uses a small accent indicator rather than a separate filled
block. Range sliders use their dedicated geometry, not text-input padding.

**Dropdown:** panel background, `1px` standard border, `10px` radius, `8px`
padding, `8px` gap from its trigger. Items are full-width, minimum `40px`
high, with `8px 10px` padding, `6px` radius and `10px` icon-to-label gap.
Use selected background on hover. Support keyboard activation, Escape to close,
and dismissal on outside interaction; return focus to the trigger on Escape.

**Disabled control:** use the same geometry, `opacity: 0.5`, no hover effect,
and native `disabled` where supported. Do not communicate unavailability
through color alone. **Pending action:** show concise status and prevent
duplicate submissions without moving surrounding elements.

**Action hierarchy:** use one primary action per operation group. Secondary
actions use the same height and padding. Destructive actions use a secondary
button with semantic error text and an explicit verb; retain existing
confirmation behavior. Icon-only actions need an accessible name and tooltip.
Place feedback immediately below the action group, using `13px` sentence-case
text with normal letter spacing, never uppercase status captions.

**Disclosure:** use a native `details`/`summary` for optional credentials.
The summary has a `40px` minimum hit area, `13px` text and an
`18px` Hugeicons indicator; use the standard focus ring. Its expanded form
uses the normal field geometry and `12px` gaps. Required first-time setup stays
visible rather than hidden inside a disclosure. In a user row, use the shared
password action button with `aria-controls` and `aria-expanded` to reveal the
`.password-editor` below the row; the name and action group stay together.

**Surface:** panel background, shared surface geometry, no gradient or shadow.
**Centered two-column layout:** cap the shared frame at `1280px`, center it with
the standard page gutters, and use a `232px` navigation column with a `24px`
content gap. An unobtrusive navigation column uses the page background without
its own border; the main content uses the shared surface recipe. At widths
≤ `700px`, stack navigation and content rather than shrinking controls.
When the header contains branding and content controls, align them to the same
two columns instead of letting the controls span the navigation column.
For a fixed header, keep its outer wrapper transparent and give the centered
grid one continuous surface with side and bottom borders shared by the brand
and player, without a top border. Use `24px` horizontal padding inside both
columns, reduced to `20px` at widths ≤ `700px`.
Keep the controls aligned to their respective columns. At the top edge, use square upper
corners and the shared radius below. Reserve its measured height plus the standard
content gap above the page, including when controls wrap.
Express the frame and responsive column placement with Tailwind grid utilities
in templates; do not duplicate these layout declarations in component CSS.
Navigation rows use `40px` minimum height, `8px 10px` padding, `10px` gaps,
`18px` icons, and `6px` radii. Use selected background for hover and selection;
keep count metadata at `12px`, aligned to the trailing edge.
**Media list:** one shared media cell: `28px` cover/placeholder, `10px` gap,
title at body size/weight `600`, artist underneath at metadata size. Use the
same cell on Tracks, Server tracks, every source, and Downloads. Allow long
titles to wrap. Row height follows content, with `8px 10px` padding and `10px`
column gaps; no fixed row height or page-specific compact density.

List headers use sentence-case `12px` secondary text. Define each list's grid
once for both its header and rows. Shared desktop column widths: playback and
download actions `32px`, source `96px`, personal added date `152px`, status
`160px`, duration `44px`; the media column takes remaining space. Omit columns
that do not apply to that page rather than inserting empty placeholder columns.
Queue rows have the media cell, source and a flexible status column with a
`160px` minimum; they do not gain playback or membership actions implicitly.

Use subtle horizontal separators, selected background on hover and for the
current track, and the same `32px` action buttons everywhere. Playback/download
controls stay discoverable on touch and keyboard; unavailable actions use the
normal disabled treatment. Source icons and labels use secondary text, not a
different brand-colored badge for every provider. Dates and durations use
tabular numerals. Retain all displayed data when adapting the layout.

When the list's available width is ≤ `700px`, use playback / media / download
columns, with metadata and status wrapping below the media cell. Use a container
query so this also works beside the desktop sidebar on intermediate screens.
Queue rows stack media, source and status. Hide
the redundant desktop header; retain labels where a value would be ambiguous.
Decorative covers may be omitted in narrow playback rows to preserve title
space; their dimensions stay unchanged wherever shown. Avoid a fixed table
minimum width that forces horizontal scrolling for routine music browsing.

**Utility list:** user and proxy rows share the same padding, separators and
action geometry. Let addresses and usernames wrap. Proxy actions may wrap below
the identity rather than squeezing fields or abbreviating action labels.
Keep the user name and its action group in one horizontal row. When space is
limited, show password/delete actions as shared icon buttons with explicit
accessible names and tooltips. Keep an expanded password editor separate from
the summary row. Proxy Ping checks YouTube reachability through that specific
proxy; Ping all reports a result for each personal proxy, including failures.
Place the result alongside its proxy identity and disable duplicate checks
while pending. Preview results must be clearly identified as demonstrations.

**Status:** use `13px` secondary text plus an `18px` icon and `6px` gap. Ready/
connected states may use primary text; errors use the shared error token. Use
literal labels such as “Ready”, “Queued”, “Downloading”, “Stopped”, “Failed”.
Progress is a `3px` accent track below the status text, with a numeric value and
an accessible label. A status is not a primary button or a decorative pill.

**Empty state:** place a left-aligned short title and, when useful, a recovery
sentence in the list's normal position. Use body text and helper sizes, a
`5px` text gap and `24px 10px` padding. Offer only an existing relevant action.
Do not replace the page with a large illustration, dashed card or marketing
copy. Preserve the page heading and applicable forms; empty is not an error.

**Helper text and errors:** use the label text size; helpers use secondary
text, errors use the semantic error color and explain how to recover. Associate
field messages with controls using `aria-describedby`; invalid fields use
`aria-invalid`. General error notices use `role="alert"`, a `3px` left error
border, and `6px 10px` padding. Do not rely on a colored border alone.

## Icons and branding

Import Hugeicons **Stroke Rounded** from the MIT-licensed
`@hugeicons/core-free-icons` package. Add named imports and symbol mappings to
`scripts/build-icons.mjs`, then run `npm run icons:build` (or
`npm run assets:build` for all generated UI assets). Never copy SVG paths or
icon data into application code or previews. `internal/web/assets/icons.svg`
is generated, committed output; do not edit it manually. Use the shared `icon`
template or references to this sprite. Preserve `docs/hugeicons-license.txt`.
Icons inherit text color; hide decorative SVGs with `aria-hidden="true"`.
Place action icons before labels. No emoji, Unicode substitutes, or CDN assets.

The brand mark is the custom static equalizer, not an action icon: a transparent
`28px × 28px` square, `1px` accent border, `7px` radius; three centered accent
bars with `3px` gaps, `3px` widths, `2px` radii and heights `10px, 17px, 10px`.
Build it with CSS, prevent shrinking, and place it `10px` before the wordmark.
Do not animate it or change its geometry between themes.

Cover placeholders retain the brand mark's `28px × 28px` frame with smaller bars:
`1px` muted border, `7px` radius, `2px` bars and gaps, bar heights `6px, 11px, 6px`,
muted bar color and `0.65` opacity. Keep the cover container dimensions unchanged;
this exception does not apply to the header logo.

## Theme control and responsive behavior

Use `mountThemeToggle(button)` from `internal/web/assets/theme-toggle.js` for
theme buttons. Show the active theme with sun/moon icons and describe the next
action in the accessible label and tooltip. There are two visible states:
Light and Dark. Persist explicit choices in `localStorage` under
`trackharbor-theme`. Without a valid saved choice, follow OS changes until a
manual toggle. Apply the initial theme before rendering; storage failure must
not prevent switching for the current page. Keep open tabs consistent.

Use the same element dimensions on desktop and narrow screens. At ≤ `700px`,
apply the padding values above; wrap action groups and stack form columns.
The navigation may collapse into a native disclosure showing the current page,
above the content. Keep one navigation tree, expand it on desktop, and retain
access to every route and account action when expanded. The fixed player must
reserve its measured wrapped height so it cannot cover navigation or content.
No overlapping controls, clipped content, or fixed heights that hide errors.
Keep actions available on touch and keyboard, not only on hover. All text,
surfaces, borders, icons, and interactive states must work in both themes.

## Consistency check

Before delivery, compare every changed element against these tables and the
same element on sibling pages. For a cross-page redesign, review Tracks,
Server tracks, Spotify, VK Music, YouTube, Downloads, all Settings sections,
and Sign in in both themes at desktop and narrow widths. Include connected/
disconnected, empty/populated, pending, disabled, error and stopped states
where relevant. Check long titles/addresses, focus visibility and wrapped
actions. Do not claim browser review when only source was inspected.

Keep previews available until review and implementation are complete, then
remove them. Reuse existing components instead of duplicating markup or theme
logic. Do not change the palette as part of an unrelated UI change.
