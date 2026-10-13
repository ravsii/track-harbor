---
name: design
description: Use when creating, reviewing, or changing TrackHarbor UI elements, styles, layouts, typography, colors, icons, spacing, or interactive states.
---

# UI Design

Use one consistent visual language across the application. These are reusable
element rules, not page specifications. Reuse the values below instead of
inventing sizes for each screen. This guide defines the target standard;
existing differences must be addressed through reviewed, focused UI changes,
not an automatic application-wide rewrite.

## Workflow

1. Understand the existing element, behavior, and dependencies first.
2. Before changing appearance, create separate temporary HTML previews with
   dummy content. Show both themes, desktop and narrow layouts, and relevant
   hover, keyboard-focus, disabled, and error states to the user.
3. Apply the reviewed design without changing unrelated behavior. Delete
   preview files and preview-only assets afterward; never commit them.

Documentation-only and appearance-neutral backend changes need no preview.
Prioritize usable controls and content over decorative copy or illustrations.

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
**List row:** shared row spacing, subtle border separator; hover uses selected
background. Keep column alignment consistent. Wrap or scroll content rather
than shrinking text or controls to fit.

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
No overlapping controls, clipped content, or fixed heights that hide errors.
Keep actions available on touch and keyboard, not only on hover. All text,
surfaces, borders, icons, and interactive states must work in both themes.

## Consistency check

Before delivery, compare every changed element against these tables, check
both themes and narrow layouts, and remove preview artifacts. Reuse existing
components instead of duplicating markup or theme logic. Do not change the
palette or enable Tailwind Preflight as part of an unrelated UI change.
